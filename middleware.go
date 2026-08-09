package traefik_x402

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	paymentRequiredHeader  = "PAYMENT-REQUIRED"
	paymentSignatureHeader = "PAYMENT-SIGNATURE"
	paymentResponseHeader  = "PAYMENT-RESPONSE"
)

type middleware struct {
	next        http.Handler
	name        string
	config      runtimeConfig
	facilitator *facilitatorClient
	inFlightMu  sync.Mutex
	inFlight    map[[sha256.Size]byte]struct{}
}

// New creates an x402 v2 Traefik middleware.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if next == nil {
		return nil, fmt.Errorf("x402 middleware requires a downstream handler")
	}

	runtime, err := normalizeConfig(config)
	if err != nil {
		return nil, fmt.Errorf("x402 middleware configuration: %w", err)
	}

	return &middleware{
		next:        next,
		name:        name,
		config:      runtime,
		facilitator: newFacilitatorClient(runtime),
		inFlight:    make(map[[sha256.Size]byte]struct{}),
	}, nil
}

func (m *middleware) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if _, allowed := m.config.allowedMethods[request.Method]; !allowed {
		writer.Header().Set("Allow", strings.Join(m.config.allowedMethodList, ", "))
		writeJSONError(writer, http.StatusMethodNotAllowed, "method is not enabled for x402 payment")
		return
	}

	required := m.paymentRequired(request, "Payment required")
	paymentHeader := request.Header.Get(paymentSignatureHeader)
	if paymentHeader == "" {
		m.writePaymentRequired(writer, required)
		return
	}

	payload, err := decodePaymentPayload(paymentHeader, m.config.maxPaymentHeaderBytes)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, "invalid payment signature")
		return
	}
	if !requirementsMatch(payload.Accepted, m.config.requirements) {
		required.Error = "No matching payment requirements"
		m.writePaymentRequired(writer, required)
		return
	}

	paymentKey, err := paymentFingerprint(payload, m.config.requirements.Network)
	if err != nil {
		writeJSONError(writer, http.StatusBadRequest, "invalid payment signature")
		return
	}
	if !m.beginPayment(paymentKey) {
		writeJSONError(writer, http.StatusConflict, "payment is already being processed")
		return
	}
	defer m.finishPayment(paymentKey)

	verification, err := m.facilitator.verify(request.Context(), payload, m.config.requirements)
	if err != nil {
		log.Printf("[%s] x402 facilitator verification error: %v", m.name, err)
		writeJSONError(writer, http.StatusBadGateway, "payment facilitator unavailable")
		return
	}
	if !verification.IsValid {
		required.Error = firstNonEmpty(verification.InvalidMessage, verification.InvalidReason, "Payment verification failed")
		m.writePaymentRequired(writer, required)
		return
	}

	forwardRequest := request.Clone(request.Context())
	forwardRequest.Header = request.Header.Clone()
	if !m.config.forwardPaymentSignature {
		forwardRequest.Header.Del(paymentSignatureHeader)
	}
	forwardRequest.Header.Del(m.config.payerHeader)
	if verification.Payer != "" {
		forwardRequest.Header.Set(m.config.payerHeader, verification.Payer)
	}

	captured := newLimitedResponseWriter(m.config.maxResponseBodyBytes)
	m.next.ServeHTTP(captured, forwardRequest)
	// The payment protocol headers are owned by this middleware. Never let a
	// protected service forge a challenge or settlement result.
	captured.Header().Del(paymentRequiredHeader)
	captured.Header().Del(paymentResponseHeader)
	if captured.exceededLimit() {
		log.Printf("[%s] x402 protected response exceeded %d bytes", m.name, m.config.maxResponseBodyBytes)
		writeJSONError(writer, http.StatusBadGateway, "protected response is too large for settlement")
		return
	}

	// x402 settlement is performed only for successful upstream responses.
	if captured.statusCode() >= http.StatusBadRequest {
		captured.flushTo(writer)
		return
	}

	settlement, err := m.facilitator.settle(request.Context(), payload, m.config.requirements)
	if err != nil {
		log.Printf("[%s] x402 facilitator settlement error: %v", m.name, err)
		writeJSONError(writer, http.StatusBadGateway, "payment settlement unavailable")
		return
	}
	if settlement.Network == "" {
		settlement.Network = m.config.requirements.Network
	}
	if settlement.Payer == "" {
		settlement.Payer = verification.Payer
	}

	encodedSettlement, err := encodeProtocolHeader(settlement)
	if err != nil {
		log.Printf("[%s] x402 settlement response encoding error: %v", m.name, err)
		writeJSONError(writer, http.StatusInternalServerError, "payment settlement response failed")
		return
	}
	if !settlement.Success {
		writer.Header().Set(paymentResponseHeader, encodedSettlement)
		exposeHeader(writer.Header(), paymentResponseHeader)
		writer.Header().Set("Cache-Control", "no-store")
		writeJSONError(writer, http.StatusPaymentRequired, "payment settlement failed")
		return
	}

	captured.Header().Set(paymentResponseHeader, encodedSettlement)
	exposeHeader(captured.Header(), paymentResponseHeader)
	disableSharedCaching(captured.Header())
	captured.flushTo(writer)
}

func (m *middleware) beginPayment(key [sha256.Size]byte) bool {
	m.inFlightMu.Lock()
	defer m.inFlightMu.Unlock()
	if _, exists := m.inFlight[key]; exists {
		return false
	}
	m.inFlight[key] = struct{}{}
	return true
}

func (m *middleware) finishPayment(key [sha256.Size]byte) {
	m.inFlightMu.Lock()
	delete(m.inFlight, key)
	m.inFlightMu.Unlock()
}

func disableSharedCaching(header http.Header) {
	header.Set("Cache-Control", "private, no-store")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
	header.Del("Surrogate-Control")
	header.Del("CDN-Cache-Control")
	header.Del("Cloudflare-CDN-Cache-Control")
}

func (m *middleware) paymentRequired(request *http.Request, message string) PaymentRequired {
	resourceURL := m.config.resourceURL
	if resourceURL == "" {
		resourceURL = externalRequestURL(request)
	}

	return PaymentRequired{
		X402Version: protocolVersion,
		Error:       message,
		Resource: &ResourceInfo{
			URL:         resourceURL,
			Description: m.config.description,
			MimeType:    m.config.mimeType,
		},
		Accepts: []PaymentRequirements{m.config.requirements},
	}
}

func (m *middleware) writePaymentRequired(writer http.ResponseWriter, required PaymentRequired) {
	encoded, err := encodeProtocolHeader(required)
	if err != nil {
		log.Printf("[%s] x402 payment requirement encoding error: %v", m.name, err)
		writeJSONError(writer, http.StatusInternalServerError, "payment requirement response failed")
		return
	}

	writer.Header().Set(paymentRequiredHeader, encoded)
	exposeHeader(writer.Header(), paymentRequiredHeader)
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusPaymentRequired, required)
}

func externalRequestURL(request *http.Request) string {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	if forwarded := firstForwardedValue(request.Header.Get("X-Forwarded-Proto")); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}

	host := request.Host
	if host == "" {
		host = request.URL.Host
	}
	path := request.URL.Path
	if path == "" {
		path = "/"
	}

	return (&url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     path,
		RawPath:  request.URL.RawPath,
		RawQuery: request.URL.RawQuery,
	}).String()
}

func firstForwardedValue(value string) string {
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		value = value[:comma]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeJSONError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, status, map[string]string{"error": message})
}

func writeJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
