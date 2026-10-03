package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct client, no header", "203.0.113.9:5000", "", "203.0.113.9"},
		{"direct client forging the header", "203.0.113.9:5000", "1.2.3.4", "203.0.113.9"},
		{"behind nginx on loopback", "127.0.0.1:5000", "198.51.100.7", "198.51.100.7"},
		{"behind nginx via docker gateway", "172.18.0.1:5000", "198.51.100.7", "198.51.100.7"},
		{"client-sent entry before the real one", "127.0.0.1:5000", "1.2.3.4, 198.51.100.7", "198.51.100.7"},
		{"proxy without the header", "127.0.0.1:5000", "", "127.0.0.1"},
		{"proxy with a garbage header", "127.0.0.1:5000", "not-an-ip", "127.0.0.1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}