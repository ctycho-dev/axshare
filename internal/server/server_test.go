package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ctycho-dev/axshare/internal/hub"
	"github.com/ctycho-dev/axshare/internal/store"
)

// newTestServerWithStore builds the real router on a SQLite store in a temp
// directory and also returns the store, for tests that need to set up or
// inspect data directly.
func newTestServerWithStore(t *testing.T) (http.Handler, *store.SQLite) {
	t.Helper()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(":0", st, hub.New(st), Config{}).Handler, st
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	h, _ := newTestServerWithStore(t)
	return h
}

func TestCreateRoomOwnership(t *testing.T) {
	h, st := newTestServerWithStore(t)
	ctx := context.Background()

	u, err := st.UpsertUser(ctx, "github", "42", store.User{Name: "Ada"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := st.CreateSession(ctx, "tok", u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Signed in: the room belongs to the user and lives 7 days.
	req := httptest.NewRequest(http.MethodPost, "/api/rooms", strings.NewReader("hi"))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
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
	owned, err := st.Get(ctx, resp.ID)
	if err != nil {
		t.Fatalf("Get owned: %v", err)
	}
	if owned.OwnerID != u.ID {
		t.Errorf("owner = %d, want %d", owned.OwnerID, u.ID)
	}
	if left := time.Until(owned.ExpiresAt); left < 6*24*time.Hour {
		t.Errorf("owned room expires in %v, want about 7 days", left)
	}

	// Anonymous: no owner, 24 hours.
	anon, err := st.Get(ctx, createRoom(t, h, "hi"))
	if err != nil {
		t.Fatalf("Get anon: %v", err)
	}
	if anon.OwnerID != 0 {
		t.Errorf("anonymous owner = %d, want 0", anon.OwnerID)
	}
	if left := time.Until(anon.ExpiresAt); left > 25*time.Hour {
		t.Errorf("anonymous room expires in %v, want about 24 hours", left)
	}
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

// doAs is do with a session cookie, so the request is signed in.
func doAs(h http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMyRooms(t *testing.T) {
	h, st := newTestServerWithStore(t)
	ctx := context.Background()

	u, err := st.UpsertUser(ctx, "github", "42", store.User{Name: "Ada"})
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := st.CreateSession(ctx, "tok", u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Anonymous: refused.
	if rec := do(h, http.MethodGet, "/api/me/rooms", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous status = %d, want 401", rec.Code)
	}

	// Signed in with no files: an empty JSON list, not null.
	rec := doAs(h, "tok", http.MethodGet, "/api/me/rooms", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("empty list body = %s, want []", got)
	}

	// One owned file and one anonymous file: only the owned one is listed.
	if rec := doAs(h, "tok", http.MethodPost, "/api/rooms", "hello"); rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want 201", rec.Code)
	}
	createRoom(t, h, "anonymous")

	rec = doAs(h, "tok", http.MethodGet, "/api/me/rooms", "")
	var list []roomInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("listed %d files, want 1", len(list))
	}
	if list[0].Bytes != 5 || list[0].Ext != store.DefaultExt || list[0].Expires == 0 {
		t.Errorf("file info = %+v, want 5 bytes, plain, non-zero expiry", list[0])
	}
}
