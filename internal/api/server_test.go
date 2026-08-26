package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"deep-pile-pour-integrity-closure/internal/domain"
)

type okStore struct{}

func (okStore) Ping(context.Context) error { return nil }
func (okStore) Close() error               { return nil }

type failingStore struct{}

func (failingStore) Ping(context.Context) error { return errors.New("down") }
func (failingStore) Close() error               { return nil }

func TestHealthOK(t *testing.T) {
	srv := NewServer(okStore{}, domain.Services{Store: okStore{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "ok" || got.DB != "ok" {
		t.Fatalf("response = %+v, want ok/ok", got)
	}
}

func TestHealthDegradedOnPingError(t *testing.T) {
	srv := NewServer(failingStore{}, domain.Services{Store: failingStore{}})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", got.Status)
	}
}

func TestHealthDegradedNilStore(t *testing.T) {
	srv := NewServer(nil, domain.Services{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
