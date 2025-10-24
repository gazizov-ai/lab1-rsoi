package main

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://program:test@localhost:5432/persons?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM persons`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	return db
}

func newTestServer(t *testing.T, db *sql.DB) http.Handler {
	t.Helper()
	r := buildRouter(db)
	return r
}

func idFromLocation(t *testing.T, w *httptest.ResponseRecorder) int {
	t.Helper()
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatalf("empty location header")
	}
	parts := strings.Split(strings.TrimRight(loc, "/"), "/")
	idStr := parts[len(parts)-1]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		t.Fatalf("bad location %q", loc)
	}
	return id
}

func TestCreate201_EmptyBody_Location(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	h := newTestServer(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons",
		bytes.NewBufferString(`{"name":"Alice","age":22}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body=%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc == "" {
		t.Fatalf("Location header is empty")
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body must be empty on 201, got: %q", w.Body.String())
	}
}

func TestGet404(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	h := newTestServer(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/persons/29042003", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", w.Code, w.Body.String())
	}
}

func TestFullFlow(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	h := newTestServer(t, db)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/persons",
		bytes.NewBufferString(`{"name":"Almas","work":"Student"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d, body=%s", w.Code, w.Body.String())
	}
	id := idFromLocation(t, w)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/persons", nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/persons/"+strconv.Itoa(id),
		bytes.NewBufferString(`{"work":"Dev"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d, body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/persons/"+strconv.Itoa(id), nil)
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d, body=%s", w.Code, w.Body.String())
	}
}
