// Command backend starts a minimal HTTP service for local gateway demonstrations.
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
		name = "users-service"
	}
	address := os.Getenv("BACKEND_ADDRESS")
	if address == "" {
		address = ":8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"message": "hello",
			"service": name,
		})
	})
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": name,
			"users": []map[string]any{
				{"id": "1", "name": "Ada"},
				{"id": "2", "name": "Linus"},
			},
		})
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": name,
			"user": map[string]string{
				"id":   r.PathValue("id"),
				"name": "Demo User",
			},
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
