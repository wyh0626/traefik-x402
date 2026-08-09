package traefik_x402

import (
	"strings"
	"testing"
)

func TestEVMFingerprintUsesSignedAuthorizationIdentity(t *testing.T) {
	payload := PaymentPayload{Payload: map[string]interface{}{
		"signature": "0x" + strings.Repeat("11", 65),
		"authorization": map[string]interface{}{
			"from":  "0x00000000000000000000000000000000000000AA",
			"nonce": "0x" + strings.Repeat("BB", 32),
		},
	}}
	first, err := paymentFingerprint(payload, "eip155:84532")
	if err != nil {
		t.Fatalf("derive EVM fingerprint: %v", err)
	}

	// Neither a differently encoded signature nor fields outside the signed
	// authorization identity may create a second in-flight payment identity.
	payload.Payload["signature"] = "0x" + strings.Repeat("22", 65)
	payload.Payload["ignored"] = "changed"
	payload.Resource = &ResourceInfo{URL: "https://attacker.example/changed"}
	second, err := paymentFingerprint(payload, "eip155:84532")
	if err != nil {
		t.Fatalf("derive changed EVM fingerprint: %v", err)
	}
	if first != second {
		t.Fatal("same EVM owner and nonce must have the same fingerprint")
	}

	payload.Payload["authorization"].(map[string]interface{})["nonce"] = "0x" + strings.Repeat("CC", 32)
	third, err := paymentFingerprint(payload, "eip155:84532")
	if err != nil {
		t.Fatalf("derive distinct EVM fingerprint: %v", err)
	}
	if first == third {
		t.Fatal("distinct EVM nonces must have different fingerprints")
	}
}

func TestPermit2FingerprintCanonicalizesNonce(t *testing.T) {
	payload := PaymentPayload{Payload: map[string]interface{}{
		"signature": "0x" + strings.Repeat("11", 65),
		"permit2Authorization": map[string]interface{}{
			"from":  "0x0000000000000000000000000000000000000002",
			"nonce": "00042",
		},
	}}
	first, err := paymentFingerprint(payload, "eip155:84532")
	if err != nil {
		t.Fatalf("derive Permit2 fingerprint: %v", err)
	}
	payload.Payload["permit2Authorization"].(map[string]interface{})["nonce"] = "42"
	second, err := paymentFingerprint(payload, "eip155:84532")
	if err != nil {
		t.Fatalf("derive canonical Permit2 fingerprint: %v", err)
	}
	if first != second {
		t.Fatal("equivalent Permit2 nonces must have the same fingerprint")
	}
}

func TestPaymentFingerprintRejectsUnknownOrMalformedShapes(t *testing.T) {
	tests := []struct {
		name    string
		network string
		payload map[string]interface{}
	}{
		{name: "missing EVM signature", network: "eip155:84532", payload: map[string]interface{}{"authorization": map[string]interface{}{}}},
		{name: "ambiguous EVM authorization", network: "eip155:84532", payload: map[string]interface{}{
			"signature":            "0x01",
			"authorization":        map[string]interface{}{},
			"permit2Authorization": map[string]interface{}{},
		}},
		{name: "unsupported SVM network", network: "solana:testnet", payload: map[string]interface{}{"transaction": "dGVzdA=="}},
		{name: "unsupported namespace", network: "bitcoin:mainnet", payload: map[string]interface{}{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := paymentFingerprint(PaymentPayload{Payload: test.payload}, test.network); err == nil {
				t.Fatal("expected malformed fingerprint payload to be rejected")
			}
		})
	}
}
