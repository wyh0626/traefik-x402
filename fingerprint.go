package traefik_x402

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

type fingerprintMaterial struct {
	Kind    string `json:"kind"`
	Network string `json:"network"`
	Owner   string `json:"owner,omitempty"`
	Nonce   string `json:"nonce,omitempty"`
}

// paymentFingerprint derives a stable identity from the scheme data that is
// actually signed. It intentionally ignores resource, extensions, and other
// outer fields that a client can change without creating a new authorization.
func paymentFingerprint(payload PaymentPayload, network string) ([sha256.Size]byte, error) {
	namespace := strings.SplitN(network, ":", 2)[0]
	var material fingerprintMaterial
	var err error

	if namespace != "eip155" {
		err = fmt.Errorf("unsupported network namespace %q", namespace)
	} else {
		material, err = evmFingerprintMaterial(payload.Payload, network)
	}
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	encoded, err := json.Marshal(material)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode fingerprint: %w", err)
	}
	return sha256.Sum256(encoded), nil
}

func evmFingerprintMaterial(payload map[string]interface{}, network string) (fingerprintMaterial, error) {
	if _, err := canonicalHexString(requiredString(payload, "signature"), 0); err != nil {
		return fingerprintMaterial{}, fmt.Errorf("invalid EVM signature: %w", err)
	}

	authorization, hasEIP3009 := payload["authorization"]
	permit2Authorization, hasPermit2 := payload["permit2Authorization"]
	if hasEIP3009 == hasPermit2 {
		return fingerprintMaterial{}, fmt.Errorf("EVM payload must contain exactly one authorization type")
	}

	if hasEIP3009 {
		authorizationMap, ok := authorization.(map[string]interface{})
		if !ok {
			return fingerprintMaterial{}, fmt.Errorf("authorization must be an object")
		}
		owner, err := canonicalHexString(requiredString(authorizationMap, "from"), 20)
		if err != nil {
			return fingerprintMaterial{}, fmt.Errorf("invalid authorization.from: %w", err)
		}
		nonce, err := canonicalHexString(requiredString(authorizationMap, "nonce"), 32)
		if err != nil {
			return fingerprintMaterial{}, fmt.Errorf("invalid authorization.nonce: %w", err)
		}
		return fingerprintMaterial{Kind: "eip3009", Network: network, Owner: owner, Nonce: nonce}, nil
	}

	authorizationMap, ok := permit2Authorization.(map[string]interface{})
	if !ok {
		return fingerprintMaterial{}, fmt.Errorf("permit2Authorization must be an object")
	}
	owner, err := canonicalHexString(requiredString(authorizationMap, "from"), 20)
	if err != nil {
		return fingerprintMaterial{}, fmt.Errorf("invalid permit2Authorization.from: %w", err)
	}
	nonceValue := requiredString(authorizationMap, "nonce")
	nonce, ok := new(big.Int).SetString(nonceValue, 10)
	if !ok || nonce.Sign() < 0 {
		return fingerprintMaterial{}, fmt.Errorf("permit2Authorization.nonce must be a non-negative decimal integer")
	}
	return fingerprintMaterial{Kind: "permit2", Network: network, Owner: owner, Nonce: nonce.String()}, nil
}

func requiredString(values map[string]interface{}, key string) string {
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func canonicalHexString(value string, expectedBytes int) (string, error) {
	if len(value) < 3 || !(strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X")) {
		return "", fmt.Errorf("must be a non-empty 0x-prefixed hex string")
	}
	decoded, err := hex.DecodeString(value[2:])
	if err != nil || len(decoded) == 0 {
		return "", fmt.Errorf("must be a non-empty 0x-prefixed hex string")
	}
	if expectedBytes > 0 && len(decoded) != expectedBytes {
		return "", fmt.Errorf("must contain %d bytes", expectedBytes)
	}
	return "0x" + hex.EncodeToString(decoded), nil
}
