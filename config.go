package traefik_x402

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	defaultFacilitatorURL                  = "https://x402.org/facilitator"
	defaultScheme                          = "exact"
	defaultMimeType                        = "application/json"
	defaultPayerHeader                     = "X-X402-Payer"
	defaultMaxTimeoutSeconds               = 60
	defaultFacilitatorTimeoutSeconds       = 15
	defaultMaxPaymentHeaderBytes           = 64 << 10
	defaultMaxResponseBodyBytes      int64 = 10 << 20
	defaultMaxFacilitatorBodyBytes   int64 = 1 << 20
)

// Config is the Traefik dynamic configuration for the x402 middleware.
type Config struct {
	FacilitatorURL            string                 `json:"facilitatorURL,omitempty"`
	FacilitatorHeaders        map[string]string      `json:"facilitatorHeaders,omitempty"`
	AllowInsecureFacilitator  bool                   `json:"allowInsecureFacilitator,omitempty"`
	FacilitatorTimeoutSeconds int                    `json:"facilitatorTimeoutSeconds,omitempty"`
	MaxFacilitatorBodyBytes   int64                  `json:"maxFacilitatorBodyBytes,omitempty"`
	Scheme                    string                 `json:"scheme,omitempty"`
	Network                   string                 `json:"network,omitempty"`
	Asset                     string                 `json:"asset,omitempty"`
	Amount                    string                 `json:"amount,omitempty"`
	PayTo                     string                 `json:"payTo,omitempty"`
	MaxTimeoutSeconds         int                    `json:"maxTimeoutSeconds,omitempty"`
	Extra                     map[string]interface{} `json:"extra,omitempty"`
	ResourceURL               string                 `json:"resourceURL,omitempty"`
	Description               string                 `json:"description,omitempty"`
	MimeType                  string                 `json:"mimeType,omitempty"`
	MaxPaymentHeaderBytes     int                    `json:"maxPaymentHeaderBytes,omitempty"`
	MaxResponseBodyBytes      int64                  `json:"maxResponseBodyBytes,omitempty"`
	AllowedMethods            []string               `json:"allowedMethods,omitempty"`
	ForwardPaymentSignature   bool                   `json:"forwardPaymentSignature,omitempty"`
	PayerHeader               string                 `json:"payerHeader,omitempty"`
}

type runtimeConfig struct {
	facilitatorURL            string
	facilitatorHeaders        map[string]string
	facilitatorTimeoutSeconds int
	maxFacilitatorBodyBytes   int64
	requirements              PaymentRequirements
	resourceURL               string
	description               string
	mimeType                  string
	maxPaymentHeaderBytes     int
	maxResponseBodyBytes      int64
	allowedMethods            map[string]struct{}
	allowedMethodList         []string
	forwardPaymentSignature   bool
	payerHeader               string
}

// CreateConfig creates the middleware configuration with safe defaults.
func CreateConfig() *Config {
	return &Config{
		FacilitatorURL:            defaultFacilitatorURL,
		FacilitatorTimeoutSeconds: defaultFacilitatorTimeoutSeconds,
		MaxFacilitatorBodyBytes:   defaultMaxFacilitatorBodyBytes,
		Scheme:                    defaultScheme,
		MaxTimeoutSeconds:         defaultMaxTimeoutSeconds,
		MimeType:                  defaultMimeType,
		MaxPaymentHeaderBytes:     defaultMaxPaymentHeaderBytes,
		MaxResponseBodyBytes:      defaultMaxResponseBodyBytes,
		AllowedMethods:            []string{http.MethodGet, http.MethodHead},
		PayerHeader:               defaultPayerHeader,
	}
}

func normalizeConfig(config *Config) (runtimeConfig, error) {
	if config == nil {
		return runtimeConfig{}, fmt.Errorf("configuration is required")
	}

	facilitatorURL := strings.TrimSpace(config.FacilitatorURL)
	if facilitatorURL == "" {
		facilitatorURL = defaultFacilitatorURL
	}
	parsedFacilitator, err := url.Parse(facilitatorURL)
	if err != nil || parsedFacilitator.Scheme == "" || parsedFacilitator.Host == "" {
		return runtimeConfig{}, fmt.Errorf("facilitatorURL must be an absolute URL")
	}
	if parsedFacilitator.User != nil || parsedFacilitator.RawQuery != "" || parsedFacilitator.Fragment != "" {
		return runtimeConfig{}, fmt.Errorf("facilitatorURL must not contain credentials, query, or fragment")
	}
	if parsedFacilitator.Scheme != "https" && !(config.AllowInsecureFacilitator && parsedFacilitator.Scheme == "http") {
		return runtimeConfig{}, fmt.Errorf("facilitatorURL must use https; set allowInsecureFacilitator only for local testing")
	}

	scheme := strings.TrimSpace(config.Scheme)
	if scheme == "" {
		scheme = defaultScheme
	}
	if scheme != defaultScheme {
		return runtimeConfig{}, fmt.Errorf("scheme %q is not supported; this middleware currently supports exact only", scheme)
	}

	network := strings.TrimSpace(config.Network)
	if strings.Count(network, ":") != 1 {
		return runtimeConfig{}, fmt.Errorf("network must use namespace:reference format")
	}
	networkParts := strings.SplitN(network, ":", 2)
	if networkParts[0] == "" || networkParts[1] == "" || containsSpaceOrControl(network) || strings.Contains(network, "*") {
		return runtimeConfig{}, fmt.Errorf("network must be a concrete namespace:reference value")
	}
	if networkParts[0] != "eip155" {
		return runtimeConfig{}, fmt.Errorf("network namespace %q is not supported; this release supports eip155 only", networkParts[0])
	}

	asset := strings.TrimSpace(config.Asset)
	if asset == "" || containsSpaceOrControl(asset) {
		return runtimeConfig{}, fmt.Errorf("asset is required and must not contain whitespace")
	}
	payTo := strings.TrimSpace(config.PayTo)
	if payTo == "" || containsSpaceOrControl(payTo) {
		return runtimeConfig{}, fmt.Errorf("payTo is required and must not contain whitespace")
	}

	amount := strings.TrimSpace(config.Amount)
	parsedAmount, ok := new(big.Int).SetString(amount, 10)
	if !ok || parsedAmount.Sign() <= 0 {
		return runtimeConfig{}, fmt.Errorf("amount must be a positive integer in atomic asset units")
	}

	maxTimeoutSeconds := config.MaxTimeoutSeconds
	if maxTimeoutSeconds == 0 {
		maxTimeoutSeconds = defaultMaxTimeoutSeconds
	}
	if maxTimeoutSeconds < 1 {
		return runtimeConfig{}, fmt.Errorf("maxTimeoutSeconds must be positive")
	}

	mimeType := strings.TrimSpace(config.MimeType)
	if mimeType == "" {
		mimeType = defaultMimeType
	}
	if strings.ContainsAny(mimeType, "\r\n") {
		return runtimeConfig{}, fmt.Errorf("mimeType contains an invalid newline")
	}

	resourceURL := strings.TrimSpace(config.ResourceURL)
	if resourceURL != "" {
		parsedResource, parseErr := url.Parse(resourceURL)
		if parseErr != nil || parsedResource.Host == "" || (parsedResource.Scheme != "http" && parsedResource.Scheme != "https") {
			return runtimeConfig{}, fmt.Errorf("resourceURL must be an absolute http or https URL")
		}
	}

	facilitatorTimeoutSeconds := config.FacilitatorTimeoutSeconds
	if facilitatorTimeoutSeconds == 0 {
		facilitatorTimeoutSeconds = defaultFacilitatorTimeoutSeconds
	}
	if facilitatorTimeoutSeconds < 1 {
		return runtimeConfig{}, fmt.Errorf("facilitatorTimeoutSeconds must be positive")
	}

	maxFacilitatorBodyBytes := config.MaxFacilitatorBodyBytes
	if maxFacilitatorBodyBytes == 0 {
		maxFacilitatorBodyBytes = defaultMaxFacilitatorBodyBytes
	}
	if maxFacilitatorBodyBytes < 1 {
		return runtimeConfig{}, fmt.Errorf("maxFacilitatorBodyBytes must be positive")
	}

	maxPaymentHeaderBytes := config.MaxPaymentHeaderBytes
	if maxPaymentHeaderBytes == 0 {
		maxPaymentHeaderBytes = defaultMaxPaymentHeaderBytes
	}
	if maxPaymentHeaderBytes < 1 {
		return runtimeConfig{}, fmt.Errorf("maxPaymentHeaderBytes must be positive")
	}

	maxResponseBodyBytes := config.MaxResponseBodyBytes
	if maxResponseBodyBytes == 0 {
		maxResponseBodyBytes = defaultMaxResponseBodyBytes
	}
	if maxResponseBodyBytes < 1 {
		return runtimeConfig{}, fmt.Errorf("maxResponseBodyBytes must be positive")
	}

	configuredMethods := config.AllowedMethods
	if len(configuredMethods) == 0 {
		configuredMethods = []string{http.MethodGet, http.MethodHead}
	}
	allowedMethods := make(map[string]struct{}, len(configuredMethods))
	allowedMethodList := make([]string, 0, len(configuredMethods))
	for _, configuredMethod := range configuredMethods {
		method := strings.ToUpper(strings.TrimSpace(configuredMethod))
		if !validHeaderName(method) {
			return runtimeConfig{}, fmt.Errorf("allowedMethods contains invalid HTTP method %q", configuredMethod)
		}
		if _, exists := allowedMethods[method]; exists {
			continue
		}
		allowedMethods[method] = struct{}{}
		allowedMethodList = append(allowedMethodList, method)
	}

	additionalHeaders := make(map[string]string, len(config.FacilitatorHeaders))
	for name, value := range config.FacilitatorHeaders {
		canonicalName := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if !validHeaderName(canonicalName) || forbiddenFacilitatorHeader(canonicalName) {
			return runtimeConfig{}, fmt.Errorf("facilitatorHeaders contains forbidden header %q", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return runtimeConfig{}, fmt.Errorf("facilitatorHeaders[%q] contains an invalid newline", name)
		}
		additionalHeaders[canonicalName] = value
	}

	payerHeader := strings.TrimSpace(config.PayerHeader)
	if payerHeader == "" {
		payerHeader = defaultPayerHeader
	}
	payerHeader = http.CanonicalHeaderKey(payerHeader)
	if !validHeaderName(payerHeader) || forbiddenFacilitatorHeader(payerHeader) ||
		strings.EqualFold(payerHeader, paymentRequiredHeader) ||
		strings.EqualFold(payerHeader, paymentSignatureHeader) ||
		strings.EqualFold(payerHeader, paymentResponseHeader) {
		return runtimeConfig{}, fmt.Errorf("payerHeader is not a safe HTTP header name")
	}

	extra, err := cloneExtra(config.Extra)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("extra must contain JSON values: %w", err)
	}

	return runtimeConfig{
		facilitatorURL:            strings.TrimRight(parsedFacilitator.String(), "/"),
		facilitatorHeaders:        additionalHeaders,
		facilitatorTimeoutSeconds: facilitatorTimeoutSeconds,
		maxFacilitatorBodyBytes:   maxFacilitatorBodyBytes,
		requirements: PaymentRequirements{
			Scheme:            scheme,
			Network:           network,
			Asset:             asset,
			Amount:            amount,
			PayTo:             payTo,
			MaxTimeoutSeconds: maxTimeoutSeconds,
			Extra:             extra,
		},
		resourceURL:             resourceURL,
		description:             config.Description,
		mimeType:                mimeType,
		maxPaymentHeaderBytes:   maxPaymentHeaderBytes,
		maxResponseBodyBytes:    maxResponseBodyBytes,
		allowedMethods:          allowedMethods,
		allowedMethodList:       allowedMethodList,
		forwardPaymentSignature: config.ForwardPaymentSignature,
		payerHeader:             payerHeader,
	}, nil
}

func cloneExtra(extra map[string]interface{}) (map[string]interface{}, error) {
	if len(extra) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func containsSpaceOrControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		char := value[i]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func forbiddenFacilitatorHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Connection", "Content-Length", "Content-Type", "Host", "Proxy-Authorization", "Proxy-Authenticate", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}
