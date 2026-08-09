package traefik_x402

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const protocolVersion = 2

// PaymentRequirements describes one payment option accepted by a protected resource.
type PaymentRequirements struct {
	Scheme            string                 `json:"scheme"`
	Network           string                 `json:"network"`
	Asset             string                 `json:"asset"`
	Amount            string                 `json:"amount"`
	PayTo             string                 `json:"payTo"`
	MaxTimeoutSeconds int                    `json:"maxTimeoutSeconds"`
	Extra             map[string]interface{} `json:"extra,omitempty"`
}

// ResourceInfo identifies the HTTP resource covered by the payment.
type ResourceInfo struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// PaymentRequired is the x402 v2 payload sent in the PAYMENT-REQUIRED header.
type PaymentRequired struct {
	X402Version int                   `json:"x402Version"`
	Error       string                `json:"error,omitempty"`
	Resource    *ResourceInfo         `json:"resource,omitempty"`
	Accepts     []PaymentRequirements `json:"accepts"`
}

// PaymentPayload is the x402 v2 payload received in the PAYMENT-SIGNATURE header.
type PaymentPayload struct {
	X402Version int                    `json:"x402Version"`
	Payload     map[string]interface{} `json:"payload"`
	Accepted    PaymentRequirements    `json:"accepted"`
	Resource    *ResourceInfo          `json:"resource,omitempty"`
	Extensions  map[string]interface{} `json:"extensions,omitempty"`
}

type verifyResponse struct {
	IsValid        bool                   `json:"isValid"`
	InvalidReason  string                 `json:"invalidReason,omitempty"`
	InvalidMessage string                 `json:"invalidMessage,omitempty"`
	Payer          string                 `json:"payer,omitempty"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// SettlementResponse is the x402 v2 payload sent in the PAYMENT-RESPONSE header.
type SettlementResponse struct {
	Success      bool                   `json:"success"`
	ErrorReason  string                 `json:"errorReason,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Payer        string                 `json:"payer,omitempty"`
	Transaction  string                 `json:"transaction"`
	Network      string                 `json:"network"`
	Amount       string                 `json:"amount,omitempty"`
	Extensions   map[string]interface{} `json:"extensions,omitempty"`
}

type facilitatorRequest struct {
	X402Version         int                 `json:"x402Version"`
	PaymentPayload      PaymentPayload      `json:"paymentPayload"`
	PaymentRequirements PaymentRequirements `json:"paymentRequirements"`
}

func encodeProtocolHeader(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func decodePaymentPayload(value string, maxBytes int) (PaymentPayload, error) {
	if value == "" {
		return PaymentPayload{}, fmt.Errorf("empty payment signature")
	}
	if len(value) > maxBytes {
		return PaymentPayload{}, fmt.Errorf("payment signature exceeds %d bytes", maxBytes)
	}

	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return PaymentPayload{}, fmt.Errorf("invalid base64: %w", err)
	}

	var payload PaymentPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return PaymentPayload{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if payload.X402Version != protocolVersion {
		return PaymentPayload{}, fmt.Errorf("unsupported x402 version %d", payload.X402Version)
	}
	if payload.Payload == nil {
		return PaymentPayload{}, fmt.Errorf("missing payment payload")
	}

	return payload, nil
}

// requirementsMatch intentionally follows the x402 v2 core matcher. The
// facilitator receives the complete configured requirements and performs the
// scheme-specific checks for maxTimeoutSeconds and extra.
func requirementsMatch(got, want PaymentRequirements) bool {
	return got.Scheme == want.Scheme &&
		got.Network == want.Network &&
		got.Asset == want.Asset &&
		got.Amount == want.Amount &&
		got.PayTo == want.PayTo
}
