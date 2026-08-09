package traefik_x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testNetwork = "eip155:84532"
	testAsset   = "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
	testPayTo   = "0x0000000000000000000000000000000000000001"
)

func TestCreateConfigDefaults(t *testing.T) {
	config := CreateConfig()

	if config.FacilitatorURL != defaultFacilitatorURL {
		t.Fatalf("unexpected facilitator default: %q", config.FacilitatorURL)
	}
	if config.Scheme != "exact" || config.MaxTimeoutSeconds != 60 {
		t.Fatalf("unexpected payment defaults: %#v", config)
	}
	if config.MaxResponseBodyBytes != 10<<20 || config.MaxPaymentHeaderBytes != 64<<10 {
		t.Fatalf("unexpected limit defaults: %#v", config)
	}
	if !reflect.DeepEqual(config.AllowedMethods, []string{http.MethodGet, http.MethodHead}) {
		t.Fatalf("unexpected allowed method defaults: %#v", config.AllowedMethods)
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		match  string
	}{
		{name: "missing network", mutate: func(c *Config) { c.Network = "" }, match: "network"},
		{name: "wildcard network", mutate: func(c *Config) { c.Network = "eip155:*" }, match: "concrete"},
		{name: "unsupported network", mutate: func(c *Config) { c.Network = "solana:testnet" }, match: "supports eip155 only"},
		{name: "missing asset", mutate: func(c *Config) { c.Asset = "" }, match: "asset"},
		{name: "invalid amount", mutate: func(c *Config) { c.Amount = "1.25" }, match: "positive integer"},
		{name: "zero amount", mutate: func(c *Config) { c.Amount = "0" }, match: "positive integer"},
		{name: "unsupported scheme", mutate: func(c *Config) { c.Scheme = "upto" }, match: "supports exact only"},
		{name: "insecure facilitator", mutate: func(c *Config) { c.FacilitatorURL = "http://facilitator.test" }, match: "must use https"},
		{name: "facilitator query", mutate: func(c *Config) { c.FacilitatorURL = "https://facilitator.test?secret=value" }, match: "query"},
		{name: "invalid resource", mutate: func(c *Config) { c.ResourceURL = "/private" }, match: "resourceURL"},
		{name: "forbidden header", mutate: func(c *Config) { c.FacilitatorHeaders = map[string]string{"Host": "evil.test"} }, match: "forbidden header"},
		{name: "newline header", mutate: func(c *Config) { c.FacilitatorHeaders = map[string]string{"Authorization": "one\r\ntwo"} }, match: "newline"},
		{name: "unsafe payer header", mutate: func(c *Config) { c.PayerHeader = paymentSignatureHeader }, match: "safe HTTP header"},
		{name: "invalid allowed method", mutate: func(c *Config) { c.AllowedMethods = []string{"GET\nX"} }, match: "invalid HTTP method"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validConfig("")
			test.mutate(config)
			_, err := New(context.Background(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), config, "test")
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("expected error containing %q, got %v", test.match, err)
			}
		})
	}
}

func TestDisallowedMethodReturns405WithoutPaymentOrUpstream(t *testing.T) {
	upstreamCalled := false
	handler := mustMiddleware(t, validConfig(""), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalled = true
	}))

	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/data", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("unexpected Allow header: %q", recorder.Header().Get("Allow"))
	}
	if recorder.Header().Get(paymentRequiredHeader) != "" || upstreamCalled {
		t.Fatal("disallowed method must not challenge for payment or call upstream")
	}
}

func TestAllowedMethodsCanOptInPost(t *testing.T) {
	config := validConfig("")
	config.AllowedMethods = []string{http.MethodPost}
	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/data", nil)
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired || recorder.Header().Get(paymentRequiredHeader) == "" {
		t.Fatalf("expected normal 402 challenge for opted-in POST, got %d", recorder.Code)
	}
}

func TestNoPaymentReturnsV2ChallengeWithoutCallingUpstream(t *testing.T) {
	upstreamCalled := false
	handler := mustMiddleware(t, validConfig(""), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalled = true
	}))

	request := httptest.NewRequest(http.MethodGet, "http://internal/data?q=weather", nil)
	request.Host = "api.example.test"
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if upstreamCalled {
		t.Fatal("upstream must not be called before payment verification")
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("payment challenge must not be cached")
	}

	required := decodeHeader[PaymentRequired](t, recorder.Header().Get(paymentRequiredHeader))
	if required.X402Version != 2 || len(required.Accepts) != 1 {
		t.Fatalf("invalid payment requirement: %#v", required)
	}
	if required.Resource == nil || required.Resource.URL != "https://api.example.test/data?q=weather" {
		t.Fatalf("unexpected resource URL: %#v", required.Resource)
	}
	if required.Accepts[0].Amount != "10000" || required.Accepts[0].Network != testNetwork {
		t.Fatalf("unexpected accepted requirement: %#v", required.Accepts[0])
	}
}

func TestMalformedPaymentReturns400(t *testing.T) {
	upstreamCalled := false
	handler := mustMiddleware(t, validConfig(""), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalled = true
	}))

	request := httptest.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	request.Header.Set(paymentSignatureHeader, "not-base64")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if upstreamCalled {
		t.Fatal("upstream must not be called for malformed payment")
	}
}

func TestMismatchedPaymentReturnsNewChallenge(t *testing.T) {
	config := validConfig("")
	payload := validPayload(t, config)
	payload.Accepted.Amount = "1"

	request := httptest.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	request.Header.Set(paymentSignatureHeader, encodeHeader(t, payload))
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, http.NotFoundHandler()).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", recorder.Code)
	}
	required := decodeHeader[PaymentRequired](t, recorder.Header().Get(paymentRequiredHeader))
	if required.Error != "No matching payment requirements" {
		t.Fatalf("unexpected challenge error: %q", required.Error)
	}
}

func TestVerifiedRequestIsForwardedThenSettled(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0, 3)
	var facilitatorRequests []facilitatorRequest

	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		events = append(events, strings.TrimPrefix(request.URL.Path, "/"))
		mu.Unlock()
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing configured facilitator authorization")
		}

		var envelope facilitatorRequest
		if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil {
			t.Errorf("decode facilitator request: %v", err)
		}
		facilitatorRequests = append(facilitatorRequests, envelope)
		writer.Header().Set("Content-Type", "application/json")

		switch request.URL.Path {
		case "/verify":
			_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xverified"}`)
		case "/settle":
			_, _ = io.WriteString(writer, `{"success":true,"transaction":"0xtx","network":"eip155:84532","payer":"0xverified","amount":"10000"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	config.FacilitatorHeaders = map[string]string{"Authorization": "Bearer test-token"}
	config.AllowedMethods = []string{http.MethodPost}
	payload := validPayload(t, config)

	upstream := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		events = append(events, "upstream")
		mu.Unlock()
		if request.Header.Get(paymentSignatureHeader) != "" {
			t.Error("payment signature must be stripped before proxying by default")
		}
		if request.Header.Get(defaultPayerHeader) != "0xverified" {
			t.Errorf("expected verified payer header, got %q", request.Header.Get(defaultPayerHeader))
		}
		writer.Header().Set("X-Upstream", "yes")
		writer.Header().Set("Cache-Control", "public, max-age=3600")
		writer.Header().Set("Surrogate-Control", "max-age=3600")
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, "protected result")
	})

	request := httptest.NewRequest(http.MethodPost, "https://api.example.test/data", strings.NewReader("request body"))
	request.Header.Set(paymentSignatureHeader, encodeHeader(t, payload))
	request.Header.Set(defaultPayerHeader, "attacker")
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, upstream).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || recorder.Body.String() != "protected result" {
		t.Fatalf("unexpected protected response: %d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Upstream") != "yes" {
		t.Fatal("upstream headers were not preserved")
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("Surrogate-Control") != "" {
		t.Fatalf("paid response must not be shared-cacheable: %v", recorder.Header())
	}
	settlement := decodeHeader[SettlementResponse](t, recorder.Header().Get(paymentResponseHeader))
	if !settlement.Success || settlement.Transaction != "0xtx" || settlement.Payer != "0xverified" {
		t.Fatalf("unexpected settlement response: %#v", settlement)
	}

	mu.Lock()
	gotEvents := append([]string(nil), events...)
	mu.Unlock()
	if !reflect.DeepEqual(gotEvents, []string{"verify", "upstream", "settle"}) {
		t.Fatalf("unexpected processing order: %#v", gotEvents)
	}
	if len(facilitatorRequests) != 2 {
		t.Fatalf("expected verify and settle requests, got %d", len(facilitatorRequests))
	}
	for _, envelope := range facilitatorRequests {
		if envelope.X402Version != 2 || !requirementsMatch(envelope.PaymentRequirements, payload.Accepted) {
			t.Fatalf("unexpected facilitator envelope: %#v", envelope)
		}
	}
}

func TestConcurrentPaymentReuseDoesNotCallUpstreamTwice(t *testing.T) {
	verifyStarted := make(chan struct{})
	releaseVerify := make(chan struct{})
	var firstVerifyOnce sync.Once
	var verifyCalls atomic.Int32
	var upstreamCalls atomic.Int32

	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/verify":
			firstVerifyOnce.Do(func() { close(verifyStarted) })
			verifyCalls.Add(1)
			<-releaseVerify
			_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xpayer"}`)
		case "/settle":
			_, _ = io.WriteString(writer, `{"success":true,"transaction":"0xtx","network":"eip155:84532"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	handler := mustMiddleware(t, config, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		_, _ = io.WriteString(writer, "ok")
	}))
	firstPayload := validPayload(t, config)
	paymentHeader := encodeHeader(t, firstPayload)

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	firstRequest.Header.Set(paymentSignatureHeader, paymentHeader)
	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(firstRecorder, firstRequest)
		close(firstDone)
	}()

	select {
	case <-verifyStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first payment did not reach verification")
	}

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	secondPayload := firstPayload
	secondPayload.Resource = &ResourceInfo{URL: "https://attacker.example/changed"}
	secondPayload.Extensions = map[string]interface{}{"changed": true}
	secondPayload.Payload["ignored"] = "outer shape changed"
	secondRequest.Header.Set(paymentSignatureHeader, encodeHeader(t, secondPayload))
	secondDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(secondRecorder, secondRequest)
		close(secondDone)
	}()

	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		close(releaseVerify)
		<-firstDone
		<-secondDone
		t.Fatal("duplicate payment request was not rejected while the first was in flight")
	}
	if secondRecorder.Code != http.StatusConflict {
		close(releaseVerify)
		<-firstDone
		t.Fatalf("expected 409 for concurrent payment reuse, got %d", secondRecorder.Code)
	}

	close(releaseVerify)
	<-firstDone
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first payment to succeed, got %d: %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	if verifyCalls.Load() != 1 {
		t.Fatalf("expected exactly one facilitator verify call, got %d", verifyCalls.Load())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected exactly one upstream call, got %d", upstreamCalls.Load())
	}
}

func TestInvalidVerificationDoesNotCallUpstream(t *testing.T) {
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, `{"invalidReason":"invalid_signature","invalidMessage":"signature is invalid"}`)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	request := paidRequest(t, config)
	recorder := httptest.NewRecorder()
	upstreamCalled := false
	mustMiddleware(t, config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalled = true
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPaymentRequired || upstreamCalled {
		t.Fatalf("expected 402 without upstream call, got %d, called=%v", recorder.Code, upstreamCalled)
	}
	required := decodeHeader[PaymentRequired](t, recorder.Header().Get(paymentRequiredHeader))
	if required.Error != "signature is invalid" {
		t.Fatalf("unexpected verification error: %q", required.Error)
	}
}

func TestFacilitatorFailureReturns502(t *testing.T) {
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, http.NotFoundHandler()).ServeHTTP(recorder, paidRequest(t, config))

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUpstreamFailureIsReturnedWithoutSettlement(t *testing.T) {
	settleCalls := 0
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/settle" {
			settleCalls++
		}
		_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xpayer"}`)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	upstream := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Upstream-Error", "yes")
		writer.Header().Set(paymentResponseHeader, "forged")
		writer.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(writer, "upstream failed")
	})
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, upstream).ServeHTTP(recorder, paidRequest(t, config))

	if recorder.Code != http.StatusTeapot || recorder.Body.String() != "upstream failed" {
		t.Fatalf("upstream error was not preserved: %d %q", recorder.Code, recorder.Body.String())
	}
	if settleCalls != 0 {
		t.Fatalf("settlement must not run after an upstream failure, got %d calls", settleCalls)
	}
	if recorder.Header().Get(paymentResponseHeader) != "" {
		t.Fatal("upstream must not be able to forge PAYMENT-RESPONSE")
	}
}

func TestSettlementFailureDiscardsProtectedResponse(t *testing.T) {
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/verify" {
			_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xpayer"}`)
			return
		}
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"errorReason":"insufficient_funds","payer":"0xpayer","network":"eip155:84532","transaction":""}`)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	upstream := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Secret", "must-not-leak")
		_, _ = io.WriteString(writer, "valuable protected content")
	})
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, upstream).ServeHTTP(recorder, paidRequest(t, config))

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "valuable") || recorder.Header().Get("X-Secret") != "" {
		t.Fatalf("protected response leaked after settlement failure: headers=%v body=%q", recorder.Header(), recorder.Body.String())
	}
	settlement := decodeHeader[SettlementResponse](t, recorder.Header().Get(paymentResponseHeader))
	if settlement.Success || settlement.ErrorReason != "insufficient_funds" {
		t.Fatalf("unexpected settlement failure: %#v", settlement)
	}
}

func TestOversizedProtectedResponseIsRejectedWithoutSettlement(t *testing.T) {
	settleCalls := 0
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/settle" {
			settleCalls++
			_, _ = io.WriteString(writer, `{"success":true,"transaction":"0xtx","network":"eip155:84532"}`)
			return
		}
		_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xpayer"}`)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	config.MaxResponseBodyBytes = 4
	upstream := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "12345")
	})
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, upstream).ServeHTTP(recorder, paidRequest(t, config))

	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "12345") {
		t.Fatalf("expected safe 502 response, got %d %q", recorder.Code, recorder.Body.String())
	}
	if settleCalls != 0 {
		t.Fatalf("settlement must not run for a truncated response, got %d calls", settleCalls)
	}
}

func TestForwardPaymentSignatureCanBeEnabled(t *testing.T) {
	facilitator := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/verify" {
			_, _ = io.WriteString(writer, `{"isValid":true,"payer":"0xpayer"}`)
			return
		}
		_, _ = io.WriteString(writer, `{"success":true,"transaction":"0xtx","network":"eip155:84532"}`)
	}))
	defer facilitator.Close()

	config := validConfig(facilitator.URL)
	config.ForwardPaymentSignature = true
	request := paidRequest(t, config)
	originalHeader := request.Header.Get(paymentSignatureHeader)
	upstream := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get(paymentSignatureHeader) != originalHeader {
			t.Error("payment signature was not forwarded")
		}
		_, _ = io.WriteString(writer, "ok")
	})
	recorder := httptest.NewRecorder()
	mustMiddleware(t, config, upstream).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func validConfig(facilitatorURL string) *Config {
	config := CreateConfig()
	config.Network = testNetwork
	config.Asset = testAsset
	config.Amount = "10000"
	config.PayTo = testPayTo
	config.Description = "Premium data"
	config.Extra = map[string]interface{}{"name": "USDC", "version": "2"}
	if facilitatorURL != "" {
		config.FacilitatorURL = facilitatorURL
		config.AllowInsecureFacilitator = strings.HasPrefix(facilitatorURL, "http://")
	}
	return config
}

func validPayload(t *testing.T, config *Config) PaymentPayload {
	t.Helper()
	runtime, err := normalizeConfig(config)
	if err != nil {
		t.Fatalf("normalize test config: %v", err)
	}
	return PaymentPayload{
		X402Version: protocolVersion,
		Payload: map[string]interface{}{
			"signature": "0x" + strings.Repeat("11", 65),
			"authorization": map[string]interface{}{
				"from":  "0x0000000000000000000000000000000000000002",
				"nonce": "0x" + strings.Repeat("22", 32),
			},
		},
		Accepted: runtime.requirements,
		Resource: &ResourceInfo{URL: "https://api.example.test/data"},
	}
}

func paidRequest(t *testing.T, config *Config) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "https://api.example.test/data", nil)
	request.Header.Set(paymentSignatureHeader, encodeHeader(t, validPayload(t, config)))
	return request
}

func mustMiddleware(t *testing.T, config *Config, next http.Handler) http.Handler {
	t.Helper()
	handler, err := New(context.Background(), next, config, "x402-test")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	return handler
}

func encodeHeader(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := encodeProtocolHeader(value)
	if err != nil {
		t.Fatalf("encode header: %v", err)
	}
	return encoded
}

func decodeHeader[T any](t *testing.T, value string) T {
	t.Helper()
	var decoded T
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64 header: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode JSON header: %v", err)
	}
	return decoded
}
