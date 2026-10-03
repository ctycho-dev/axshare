package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiter keeps one token bucket per client IP. Buckets are created on
// first sight and dropped after idle, so the map cannot grow without bound.
type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*bucket
	r       rate.Limit // tokens per second
	burst   int
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

func newRateLimiter(perSecond float64, burst int) *rateLimiter {
	rl := &rateLimiter{
		clients: make(map[string]*bucket),
		r:       rate.Limit(perSecond),
		burst:   burst,
	}
	go rl.sweep(time.Minute, 3*time.Minute)
	return rl
}

// limiter returns the bucket for ip, creating it on first use.
func (rl *rateLimiter) limiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.clients[ip]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(rl.r, rl.burst)}
		rl.clients[ip] = b
	}
	b.seen = time.Now()
	return b.lim
}

// sweep drops buckets not seen for idle. Same ticker shape as RunExpiry,
// minus the context: the limiter lives as long as the process.
func (rl *rateLimiter) sweep(every, idle time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for now := range t.C {
		rl.mu.Lock()
		for ip, b := range rl.clients {
			if now.Sub(b.seen) > idle {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// clientIP returns the address to rate-limit on.
//
// A client that connects directly is identified by its TCP peer address.
// When the peer is a local reverse proxy (loopback, or a private address
// such as Docker's bridge gateway), the real client is the LAST entry of
// X-Forwarded-For: that is the one the proxy appended itself. Earlier
// entries are whatever the client sent and cannot be trusted.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	peer = peer.Unmap()
	if !peer.IsLoopback() && !peer.IsPrivate() {
		// Not behind our proxy: ignore the header, it could be forged.
		return host
	}

	vals := r.Header.Values("X-Forwarded-For")
	if len(vals) == 0 {
		return host
	}
	last := vals[len(vals)-1]
	if i := strings.LastIndexByte(last, ','); i >= 0 {
		last = last[i+1:]
	}
	last = strings.TrimSpace(last)
	if _, err := netip.ParseAddr(last); err != nil {
		return host
	}
	return last
}

// middleware answers 429 when the client's bucket is empty.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.limiter(clientIP(r)).Allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}