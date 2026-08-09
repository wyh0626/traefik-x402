package traefik_x402

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type facilitatorClient struct {
	baseURL      string
	httpClient   *http.Client
	headers      map[string]string
	maxBodyBytes int64
}

type verifyResponseEnvelope struct {
	IsValid        *bool                  `json:"isValid"`
	InvalidReason  string                 `json:"invalidReason,omitempty"`
	InvalidMessage string                 `json:"invalidMessage,omitempty"`
	Payer          string                 `json:"payer,omitempty"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

type settleResponseEnvelope struct {
	Success      *bool                  `json:"success"`
	ErrorReason  string                 `json:"errorReason,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Payer        string                 `json:"payer,omitempty"`
	Transaction  *string                `json:"transaction"`
	Network      *string                `json:"network"`
	Amount       string                 `json:"amount,omitempty"`
	Extensions   map[string]interface{} `json:"extensions,omitempty"`
}

func newFacilitatorClient(config runtimeConfig) *facilitatorClient {
	return &facilitatorClient{
		baseURL: config.facilitatorURL,
		httpClient: &http.Client{
			Timeout: time.Duration(config.facilitatorTimeoutSeconds) * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		headers:      config.facilitatorHeaders,
		maxBodyBytes: config.maxFacilitatorBodyBytes,
	}
}

func (c *facilitatorClient) verify(ctx context.Context, payload PaymentPayload, requirements PaymentRequirements) (verifyResponse, error) {
	status, body, err := c.post(ctx, "/verify", facilitatorRequest{
		X402Version:         protocolVersion,
		PaymentPayload:      payload,
		PaymentRequirements: requirements,
	})
	if err != nil {
		return verifyResponse{}, err
	}

	var envelope verifyResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return verifyResponse{}, fmt.Errorf("facilitator verify returned invalid JSON: %w", err)
	}

	if status != http.StatusOK {
		if envelope.InvalidReason == "" {
			return verifyResponse{}, fmt.Errorf("facilitator verify returned HTTP %d", status)
		}
		return verifyResponse{
			IsValid:        false,
			InvalidReason:  envelope.InvalidReason,
			InvalidMessage: envelope.InvalidMessage,
			Payer:          envelope.Payer,
			Extensions:     envelope.Extensions,
		}, nil
	}
	if envelope.IsValid == nil {
		return verifyResponse{}, fmt.Errorf("facilitator verify response is missing isValid")
	}

	return verifyResponse{
		IsValid:        *envelope.IsValid,
		InvalidReason:  envelope.InvalidReason,
		InvalidMessage: envelope.InvalidMessage,
		Payer:          envelope.Payer,
		Extensions:     envelope.Extensions,
	}, nil
}

func (c *facilitatorClient) settle(ctx context.Context, payload PaymentPayload, requirements PaymentRequirements) (SettlementResponse, error) {
	status, body, err := c.post(ctx, "/settle", facilitatorRequest{
		X402Version:         protocolVersion,
		PaymentPayload:      payload,
		PaymentRequirements: requirements,
	})
	if err != nil {
		return SettlementResponse{}, err
	}

	var envelope settleResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return SettlementResponse{}, fmt.Errorf("facilitator settle returned invalid JSON: %w", err)
	}

	if status != http.StatusOK {
		if envelope.ErrorReason == "" {
			return SettlementResponse{}, fmt.Errorf("facilitator settle returned HTTP %d", status)
		}
		return settlementFromEnvelope(envelope, false), nil
	}
	if envelope.Success == nil || envelope.Transaction == nil || envelope.Network == nil {
		return SettlementResponse{}, fmt.Errorf("facilitator settle response is missing required fields")
	}

	return settlementFromEnvelope(envelope, *envelope.Success), nil
}

func settlementFromEnvelope(envelope settleResponseEnvelope, success bool) SettlementResponse {
	transaction := ""
	if envelope.Transaction != nil {
		transaction = *envelope.Transaction
	}
	network := ""
	if envelope.Network != nil {
		network = *envelope.Network
	}

	return SettlementResponse{
		Success:      success,
		ErrorReason:  envelope.ErrorReason,
		ErrorMessage: envelope.ErrorMessage,
		Payer:        envelope.Payer,
		Transaction:  transaction,
		Network:      network,
		Amount:       envelope.Amount,
		Extensions:   envelope.Extensions,
	}
}

func (c *facilitatorClient) post(ctx context.Context, endpoint string, value interface{}) (int, []byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return 0, nil, fmt.Errorf("encode facilitator request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("create facilitator request: %w", err)
	}
	for name, headerValue := range c.headers {
		request.Header.Set(name, headerValue)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("call facilitator: %w", err)
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, c.maxBodyBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, fmt.Errorf("read facilitator response: %w", err)
	}
	if int64(len(responseBody)) > c.maxBodyBytes {
		return 0, nil, fmt.Errorf("facilitator response exceeds %d bytes", c.maxBodyBytes)
	}

	return response.StatusCode, responseBody, nil
}
