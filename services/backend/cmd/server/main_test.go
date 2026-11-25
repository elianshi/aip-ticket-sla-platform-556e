package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndStats(t *testing.T) {
	h := newMux(NewAppStore(), "")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected stats 200, got %d", rec2.Code)
	}
}

func TestCreateAndTransitionTicket(t *testing.T) {
	h := newMux(NewAppStore(), "")
	body := []byte(`{"title":"核心网关 502 突增","service_id":"svc-gateway","severity":"P0","assignee":"Titan"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tickets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created Ticket
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Severity != "P0" || created.Status != "open" {
		t.Fatalf("unexpected ticket: %+v", created)
	}

	trBody := []byte(`{"status":"resolved","actor":"Titan","comment":"扩容恢复"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/tickets/"+created.ID+"/transition", bytes.NewReader(trBody))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var updated Ticket
	_ = json.NewDecoder(rec2.Body).Decode(&updated)
	if updated.Status != "resolved" || len(updated.Timeline) < 2 {
		t.Fatalf("expected resolved status & timeline, got %+v", updated)
	}
}

func TestServiceStatusToggle(t *testing.T) {
	h := newMux(NewAppStore(), "")
	body := []byte(`{"status":"outage"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/services/svc-gateway/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
