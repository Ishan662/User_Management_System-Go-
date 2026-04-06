package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/go-chi/chi/v5"
)

func TestHealthRoute(t *testing.T) {
	r :=chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, r *http.Request){
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", res.StatusCode)
	}

}