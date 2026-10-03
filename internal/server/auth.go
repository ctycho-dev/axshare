package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/ctycho-dev/axshare/internal/store"
)

const (
	sessionCookie = "axshare_session"
	stateCookie   = "axshare_oauth_state"
	sessionTTL    = 30 * 24 * time.Hour
	stateTTL      = 10 * time.Minute
)

// profile is what we keep from a provider's "who is this" response.
type profile struct {
	ID        string
	Name      string
	Email     string
	AvatarURL string
}

// provider is one way to sign in. name appears in the URLs
// (/auth/{name}/login) and in the identities table.
type provider struct {
	name  string
	oauth *oauth2.Config
	fetch func(ctx context.Context, client *http.Client) (profile, error)
}

// newProviders builds the providers that have credentials configured, in
// the order the login dialog lists them.
func newProviders(cfg Config) []*provider {
	var out []*provider
	if cfg.GitHubEnabled() {
		out = append(out, &provider{
			name: "github",
			oauth: &oauth2.Config{
				ClientID:     cfg.GitHubClientID,
				ClientSecret: cfg.GitHubClientSecret,
				Endpoint:     endpoints.GitHub,
				RedirectURL:  cfg.BaseURL + "/auth/github/callback",
				// No scopes: the default grant is the public profile.
			},
			fetch: fetchGitHubProfile,
		})
	}
	if cfg.GoogleEnabled() {
		out = append(out, &provider{
			name: "google",
			oauth: &oauth2.Config{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				Endpoint:     endpoints.Google,
				RedirectURL:  cfg.BaseURL + "/auth/google/callback",
				Scopes:       []string{"openid", "email", "profile"},
			},
			fetch: fetchGoogleProfile,
		})
	}
	return out
}

// provider returns the configured provider with that name, or nil.
func (s *Server) provider(name string) *provider {
	for _, p := range s.providers {
		if p.name == name {
			return p
		}
	}
	return nil
}

// secureCookies reports whether cookies should be HTTPS-only. True in
// production; false on http://localhost, where a Secure cookie may be
// refused by the browser.
func (s *Server) secureCookies() bool {
	return strings.HasPrefix(s.cfg.BaseURL, "https://")
}

// handleLogin: GET /auth/{provider}/login. Sends the browser to the provider.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	p := s.provider(r.PathValue("provider"))
	if p == nil {
		http.NotFound(w, r)
		return
	}

	state, err := newID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/auth",
		MaxAge:   int(stateTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, p.oauth.AuthCodeURL(state), http.StatusFound)
}

// handleCallback: GET /auth/{provider}/callback. The provider sends the
// browser here with ?code=...&state=... after the user approves.
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	p := s.provider(r.PathValue("provider"))
	if p == nil {
		http.NotFound(w, r)
		return
	}

	c, err := r.Cookie(stateCookie)
	if err != nil || c.Value == "" || r.URL.Query().Get("state") != c.Value {
		http.Error(w, "sign-in expired or was started in another browser, try again", http.StatusBadRequest)
		return
	}
	// The state is single-use: clear it whatever happens next.
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Path:     "/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		// The user pressed Cancel on the provider's page.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tok, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		log.Printf("%s callback: exchange: %v", p.name, err)
		http.Error(w, "sign-in failed", http.StatusBadGateway)
		return
	}
	prof, err := p.fetch(ctx, p.oauth.Client(ctx, tok))
	if err != nil {
		log.Printf("%s callback: %v", p.name, err)
		http.Error(w, "sign-in failed", http.StatusBadGateway)
		return
	}

	user, err := s.store.UpsertUser(ctx, p.name, prof.ID, store.User{
		Name:      prof.Name,
		Email:     prof.Email,
		AvatarURL: prof.AvatarURL,
	})
	if err != nil {
		log.Printf("%s callback: %v", p.name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := s.startSession(ctx, w, user.ID); err != nil {
		log.Printf("%s callback: %v", p.name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// startSession creates a session for userID and sets its cookie.
func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, userID int64) error {
	token, err := newID()
	if err != nil {
		return fmt.Errorf("start session: %w", err)
	}
	expires := time.Now().Add(sessionTTL)
	if err := s.store.CreateSession(ctx, token, userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// handleLogout: POST /auth/logout. Deletes the session and its cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.store.DeleteSession(r.Context(), c.Value); err != nil {
			log.Printf("logout: %v", err)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// getJSON GETs url with client and decodes the JSON body into v. client
// already carries the provider's access token.
func getJSON(ctx context.Context, client *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

func fetchGitHubProfile(ctx context.Context, client *http.Client) (profile, error) {
	var u struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := getJSON(ctx, client, "https://api.github.com/user", &u); err != nil {
		return profile{}, fmt.Errorf("github user: %w", err)
	}
	if u.ID == 0 {
		return profile{}, errors.New("github user: no id in response")
	}
	name := u.Name
	if name == "" {
		name = u.Login
	}
	return profile{
		ID:        strconv.FormatInt(u.ID, 10),
		Name:      name,
		Email:     u.Email,
		AvatarURL: u.AvatarURL,
	}, nil
}

func fetchGoogleProfile(ctx context.Context, client *http.Client) (profile, error) {
	var u struct {
		Sub           string `json:"sub"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Picture       string `json:"picture"`
	}
	if err := getJSON(ctx, client, "https://openidconnect.googleapis.com/v1/userinfo", &u); err != nil {
		return profile{}, fmt.Errorf("google userinfo: %w", err)
	}
	if u.Sub == "" {
		return profile{}, errors.New("google userinfo: no sub in response")
	}
	// Keep the email only if Google has verified it.
	email := ""
	if u.EmailVerified {
		email = u.Email
	}
	name := u.Name
	if name == "" {
		name = u.Email
	}
	return profile{ID: u.Sub, Name: name, Email: email, AvatarURL: u.Picture}, nil
}

// userKey is the context key for the signed-in user. An unexported struct
// type cannot collide with a key from any other package.
type userKey struct{}

// withUser loads the signed-in user, if any, into the request context.
// Anonymous requests pass through untouched.
func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			// No session cookie: anonymous.
			next.ServeHTTP(w, r)
			return
		}

		u, err := s.store.UserBySession(r.Context(), c.Value)
		if err != nil {
			// ErrNotFound is the normal "expired or signed out" case.
			// Anything else is a real failure worth logging.
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("withUser: %v", err)
			}
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), userKey{}, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userFrom returns the signed-in user, or false for an anonymous request.
func userFrom(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userKey{}).(store.User)
	return u, ok
}

type meResponse struct {
	SignedIn  bool     `json:"signed_in"`
	Name      string   `json:"name,omitempty"`
	AvatarURL string   `json:"avatar_url,omitempty"`
	Providers []string `json:"providers"`
}

// handleMe: GET /api/me. Tells the page whether someone is signed in and
// which sign-in providers are available. Always 200, so an anonymous visit
// does not log an error in the console.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := meResponse{Providers: make([]string, 0, len(s.providers))}
	for _, p := range s.providers {
		resp.Providers = append(resp.Providers, p.name)
	}
	if u, ok := userFrom(r.Context()); ok {
		resp.SignedIn = true
		resp.Name = u.Name
		resp.AvatarURL = u.AvatarURL
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("me: encode: %v", err)
	}
}
