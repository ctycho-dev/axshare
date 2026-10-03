package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctycho-dev/axshare/internal/hub"
	"github.com/ctycho-dev/axshare/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(":0", st, hub.New(st), Config{}).Handler
}

func TestHealth(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if !strings.Contains(rec.Body.String(), `"clients":0`) {
		t.Errorf("body = %s, want clients:0", rec.Body.String())
	}
}

func TestCreateAndGetRoom(t *testing.T) {
	h := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/rooms", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("POST returned empty id")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/rooms/"+resp.ID, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "hello" {
		t.Errorf("GET body = %q, want %q", got, "hello")
	}

	// 4. Unknown id.
	req = httptest.NewRequest(http.MethodGet, "/api/rooms/nope", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET unknown status = %d, want 404", rec.Code)
	}
}

// do sends one request to the handler and returns the recorded response.
func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// createRoom POSTs content and returns the new room's id.
func createRoom(t *testing.T, h http.Handler, content string) string {
	t.Helper()
	rec := do(h, http.MethodPost, "/api/rooms", content)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" {
		t.Fatal("POST returned empty id")
	}
	return resp.ID
}

func TestSetExt(t *testing.T) {
	h := newTestServer(t)
	id := createRoom(t, h, "package main")
	path := "/api/rooms/" + id

	// A new room reports the default file type and an expiry.
	rec := do(h, http.MethodGet, path, "")
	if got := rec.Header().Get("X-Room-Ext"); got != store.DefaultExt {
		t.Errorf("new room X-Room-Ext = %q, want %q", got, store.DefaultExt)
	}
	if got := rec.Header().Get("X-Room-Expires"); got == "" {
		t.Error("X-Room-Expires header is missing")
	}

	// Setting it returns 204 and the next GET reports it.
	rec = do(h, http.MethodPatch, path, `{"ext":"go"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	rec = do(h, http.MethodGet, path, "")
	if got := rec.Header().Get("X-Room-Ext"); got != "go" {
		t.Errorf("X-Room-Ext after PATCH = %q, want %q", got, "go")
	}

	// Bad bodies are rejected and leave the file type alone.
	for _, body := range []string{`{"ext":"GO!"}`, `{"ext":""}`, `{}`, `not json`} {
		rec = do(h, http.MethodPatch, path, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PATCH %s status = %d, want 400", body, rec.Code)
		}
	}
	rec = do(h, http.MethodGet, path, "")
	if got := rec.Header().Get("X-Room-Ext"); got != "go" {
		t.Errorf("X-Room-Ext after bad PATCHes = %q, want %q", got, "go")
	}

	// Unknown room.
	rec = do(h, http.MethodPatch, "/api/rooms/nope", `{"ext":"go"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH unknown status = %d, want 404", rec.Code)
	}
}
