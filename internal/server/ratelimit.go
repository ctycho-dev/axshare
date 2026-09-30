package server

import (
	"net"
	"net/http"
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

// middleware answers 429 when the client's bucket is empty. WebSocket
// upgrades pass through: they are one request that then lives for hours,
// and the read limit on the socket does the throttling there.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !rl.limiter(ip).Allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
