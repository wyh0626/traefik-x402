package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/verify", handleVerify)
	mux.HandleFunc("/settle", handleSettle)
	mux.HandleFunc("/protected", handleProtected)

	address := os.Getenv("MOCK_ADDR")
	if address == "" {
		address = "127.0.0.1:19090"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("mock facilitator and upstream listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func handleVerify(writer http.ResponseWriter, request *http.Request) {
	if !validFacilitatorRequest(writer, request) {
		return
	}
	log.Print("verify")
	writeJSON(writer, map[string]interface{}{
		"isValid": true,
		"payer":   "0xverified",
	})
}

func handleSettle(writer http.ResponseWriter, request *http.Request) {
	if !validFacilitatorRequest(writer, request) {
		return
	}
	log.Print("settle")
	writeJSON(writer, map[string]interface{}{
		"success":     true,
		"transaction": "0xmock-transaction",
		"network":     "eip155:84532",
		"payer":       "0xverified",
		"amount":      "10000",
	})
}

func handleProtected(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("PAYMENT-SIGNATURE") != "" {
		http.Error(writer, "payment signature was not stripped", http.StatusInternalServerError)
		return
	}
	if request.Header.Get("X-X402-Payer") != "0xverified" {
		http.Error(writer, "verified payer was not injected", http.StatusInternalServerError)
		return
	}

	log.Print("upstream")
	writer.Header().Set("Content-Type", "application/json")
	writeJSON(writer, map[string]interface{}{"ok": true})
}

func validFacilitatorRequest(writer http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodPost {
		http.Error(writer, "POST required", http.StatusMethodNotAllowed)
		return false
	}
	var envelope struct {
		X402Version int `json:"x402Version"`
	}
	if err := json.NewDecoder(request.Body).Decode(&envelope); err != nil || envelope.X402Version != 2 {
		http.Error(writer, "invalid x402 facilitator envelope", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(writer http.ResponseWriter, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
