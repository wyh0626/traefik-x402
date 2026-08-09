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

	x402 "github.com/x402-foundation/x402/go/v2"
	x402http "github.com/x402-foundation/x402/go/v2/http"
	exactevm "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/client"
	evmsigners "github.com/x402-foundation/x402/go/v2/signers/evm"
)

func main() {
	endpoint := os.Getenv("SERVER_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:18080/paid"
	}
	if len(os.Args) == 2 && os.Args[1] == "--inspect" {
		inspect(endpoint)
		return
	}

	privateKey := os.Getenv("EVM_PRIVATE_KEY")
	if privateKey == "" {
		fatalf("EVM_PRIVATE_KEY is required")
	}

	signer, err := evmsigners.NewClientSignerFromPrivateKey(privateKey)
	if err != nil {
		fatalf("create signer: %v", err)
	}
	x402Client := x402.Newx402Client().
		Register("eip155:*", exactevm.NewExactEvmScheme(signer, nil))
	httpClient := x402http.WrapHTTPClientWithPayment(
		&http.Client{Timeout: 90 * time.Second},
		x402http.Newx402HTTPClient(x402Client),
	)

	response, err := httpClient.Get(endpoint)
	if err != nil {
		fatalf("request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fatalf("read response: %v", err)
	}

	fmt.Printf("HTTP %d\n%s\n", response.StatusCode, body)
	encoded := response.Header.Get("PAYMENT-RESPONSE")
	if encoded == "" {
		fatalf("response is missing PAYMENT-RESPONSE")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fatalf("decode PAYMENT-RESPONSE: %v", err)
	}
	var settlement map[string]interface{}
	if err := json.Unmarshal(decoded, &settlement); err != nil {
		fatalf("parse PAYMENT-RESPONSE: %v", err)
	}
	pretty, _ := json.MarshalIndent(settlement, "", "  ")
	fmt.Printf("PAYMENT-RESPONSE:\n%s\n", pretty)
	if transaction, ok := settlement["transaction"].(string); ok && transaction != "" {
		fmt.Printf("BaseScan: https://sepolia.basescan.org/tx/%s\n", transaction)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		os.Exit(1)
	}
}

func inspect(endpoint string) {
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		fatalf("inspect challenge: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPaymentRequired {
		fatalf("expected 402 challenge, got %d", response.StatusCode)
	}
	encoded := response.Header.Get("PAYMENT-REQUIRED")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		fatalf("decode PAYMENT-REQUIRED: %v", err)
	}
	var challenge struct {
		Accepts []struct {
			Network string `json:"network"`
			Asset   string `json:"asset"`
			Amount  string `json:"amount"`
			PayTo   string `json:"payTo"`
		} `json:"accepts"`
	}
	if err := json.Unmarshal(decoded, &challenge); err != nil {
		fatalf("parse PAYMENT-REQUIRED: %v", err)
	}
	if len(challenge.Accepts) == 0 {
		fatalf("PAYMENT-REQUIRED has no accepted payment option")
	}
	requirement := challenge.Accepts[0]
	fmt.Printf("  network:  %s\n", requirement.Network)
	fmt.Printf("  asset:    %s\n", requirement.Asset)
	fmt.Printf("  amount:   %s atomic units", requirement.Amount)
	if strings.EqualFold(requirement.Asset, "0x036CbD53842c5426634e7929541eC2318f3dCF7e") && requirement.Amount == "1000" {
		fmt.Print(" (0.001 test USDC)")
	}
	fmt.Println()
	fmt.Printf("  receiver: %s\n", requirement.PayTo)
}

func fatalf(format string, values ...interface{}) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", values...)
	os.Exit(1)
}
