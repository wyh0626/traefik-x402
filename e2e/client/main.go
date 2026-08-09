package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultEndpoint = "http://127.0.0.1:18080/protected"
	network         = "eip155:84532"
	asset           = "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
	payTo           = "0x0000000000000000000000000000000000000001"
)

type paymentRequirements struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	Asset             string                 `json:"asset"`
	Amount            string                 `json:"amount"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
}

type paymentPayload struct {
	X402Version int                    `json:"x402Version"`
	Payload     map[string]interface{} `json:"payload"`
	Accepted    paymentRequirements    `json:"accepted"`
}

func main() {
	endpoint := os.Getenv("SERVER_URL")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	client := &http.Client{Timeout: 10 * time.Second}
	unpaid, err := client.Get(endpoint)
	if err != nil {
		fatalf("unpaid request: %v", err)
	}
	_, _ = io.Copy(io.Discard, unpaid.Body)
	_ = unpaid.Body.Close()
	if unpaid.StatusCode != http.StatusPaymentRequired || unpaid.Header.Get("PAYMENT-REQUIRED") == "" {
		fatalf("expected 402 with PAYMENT-REQUIRED, got %d", unpaid.StatusCode)
	}
	fmt.Println("PASS unpaid request: 402 + PAYMENT-REQUIRED")

	payload := paymentPayload{
		X402Version: 2,
		Payload: map[string]interface{}{
			"signature": "0x" + strings.Repeat("11", 65),
			"authorization": map[string]interface{}{
				"from":  "0x0000000000000000000000000000000000000002",
				"nonce": "0x" + strings.Repeat("22", 32),
			},
		},
		Accepted: paymentRequirements{
			Scheme:            "exact",
			Network:           network,
			Asset:             asset,
			Amount:            "10000",
			PayTo:             payTo,
			MaxTimeoutSeconds: 60,
			Extra: map[string]interface{}{
				"name":    "USDC",
				"version": "2",
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		fatalf("encode mock payment: %v", err)
	}

	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		fatalf("create paid request: %v", err)
	}
	request.Header.Set("PAYMENT-SIGNATURE", base64.StdEncoding.EncodeToString(encoded))
	paid, err := client.Do(request)
	if err != nil {
		fatalf("paid request: %v", err)
	}
	body, err := io.ReadAll(paid.Body)
	_ = paid.Body.Close()
	if err != nil {
		fatalf("read paid response: %v", err)
	}
	if paid.StatusCode != http.StatusOK {
		fatalf("expected paid request 200, got %d: %s", paid.StatusCode, strings.TrimSpace(string(body)))
	}
	if paid.Header.Get("PAYMENT-RESPONSE") == "" {
		fatalf("paid response is missing PAYMENT-RESPONSE")
	}
	if !strings.Contains(string(body), `"ok":true`) {
		fatalf("unexpected paid body: %s", strings.TrimSpace(string(body)))
	}
	fmt.Println("PASS paid request: verify -> upstream -> settle -> 200")
	fmt.Println("PASS PAYMENT-RESPONSE returned to the client")
}

func fatalf(format string, values ...interface{}) {
	fmt.Fprintf(os.Stderr, "FAIL "+format+"\n", values...)
	os.Exit(1)
}
