// Command billing starts a minimal billing service for gateway demonstrations.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		name = "billing-service"
	}
	address := os.Getenv("BACKEND_ADDRESS")
	if address == "" {
		address = ":8091"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /invoices", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": name,
			"invoices": []map[string]any{
				{"id": "inv-1001", "status": "paid", "amount": 1250},
				{"id": "inv-1002", "status": "open", "amount": 4900},
			},
		})
	})
	mux.HandleFunc("GET /invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": name,
			"invoice": map[string]any{
				"id":     r.PathValue("id"),
				"status": "open",
				"amount": 4900,
			},
		})
	})
	mux.HandleFunc("POST /payments", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"service": name,
			"status":  "processing",
		})
	})

	log.Printf("%s listening on %s", name, address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
