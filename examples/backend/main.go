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
		name = "example-backend"
	}
	http.HandleFunc("/hello", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "hello", "service": name})
	})
	log.Fatal(http.ListenAndServe(":8081", nil))
}
