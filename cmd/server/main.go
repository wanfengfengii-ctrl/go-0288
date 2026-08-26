// Command server is the executable entry point for the deep-water pile-cap
// pile-pouring integrity-closure backend. It opens the SQLite store, wires the
// service components and starts the HTTP API.
package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"deep-pile-pour-integrity-closure/internal/api"
	"deep-pile-pour-integrity-closure/internal/domain"
	"deep-pile-pour-integrity-closure/internal/service"
	"deep-pile-pour-integrity-closure/internal/store"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "piles.db"
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.New(st)
	services := domain.Services{
		Design:   svc,
		Trace:    svc,
		Material: svc,
		Evidence: svc,
		Arbiter:  svc,
		Store:    st,
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: api.NewServer(st, services).Handler(),
	}

	log.Printf("deep-pile-pour-integrity-closure listening on %s (db=%s)", addr, dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server error: %v", err)
	}
}
