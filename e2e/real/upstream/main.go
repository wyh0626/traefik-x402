package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, map[string]interface{}{"ok": true})
	})
	mux.HandleFunc("GET /paid", func(writer http.ResponseWriter, request *http.Request) {
		payer := request.Header.Get("X-X402-Payer")
		if payer == "" {
			http.Error(writer, "trusted payer header is missing", http.StatusInternalServerError)
			return
		}
		if request.Header.Get("PAYMENT-SIGNATURE") != "" {
			http.Error(writer, "payment signature was not stripped", http.StatusInternalServerError)
			return
		}

		log.Printf("paid upstream reached by verified payer %s", payer)
		writeJSON(writer, map[string]interface{}{
			"ok":      true,
			"message": "real x402 payment verified and settled",
			"payer":   payer,
		})
	})

	server := &http.Server{
		Addr:              "127.0.0.1:19091",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("real-test upstream listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(writer http.ResponseWriter, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
