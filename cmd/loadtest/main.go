// Command loadtest opens many WebSocket clients against an axshare instance
// and measures how long an edit takes to reach the other clients in its
// room.
//
// Run it against a test instance started with -no-ratelimit, never against
// production.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type config struct {
	baseURL  string
	rooms    int
	clients  int // per room: one writer, the rest readers
	interval time.Duration
	size     int
	duration time.Duration
}

// stats is shared by every client goroutine, so each field is either atomic
// or guarded by mu.
type stats struct {
	connected   atomic.Int64
	connectErrs atomic.Int64
	sent        atomic.Int64
	sendErrs    atomic.Int64
	received    atomic.Int64

	mu        sync.Mutex
	latencies []time.Duration
}

// record stores the latency of one delivered edit.
func (s *stats) record(d time.Duration) {
	s.received.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latencies = append(s.latencies, d)
}

// payload builds one edit: the send time in nanoseconds on the first line,
// then padding up to size bytes.
func payload(size int) []byte {
	head := strconv.FormatInt(time.Now().UnixNano(), 10) + "\n"
	pad := max(size-len(head), 0)
	return append([]byte(head), bytes.Repeat([]byte("x"), pad)...)
}

// sentAt reads the send time back out of a payload. ok is false for frames
// that are not load-test edits, such as a room's initial content.
func sentAt(data []byte) (t time.Time, ok bool) {
	i := bytes.IndexByte(data, '\n')
	if i <= 0 {
		return time.Time{}, false
	}
	ns, err := strconv.ParseInt(string(data[:i]), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, ns), true
}

// createRoom makes one empty room and returns its id.
func createRoom(ctx context.Context, baseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/rooms", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// dial opens a WebSocket to room id. http:// becomes ws://, https:// wss://.
func dial(ctx context.Context, baseURL, id string) (*websocket.Conn, error) {
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws/" + id

	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(dctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	// The client library refuses frames over 32 KiB unless told otherwise.
	conn.SetReadLimit(2 << 20)
	return conn, nil
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// jitter returns a random duration below d. Writers use it so they do not
// all fire at the same instant.
func jitter(d time.Duration) time.Duration {
	return rand.N(d)
}

// reader connects to room id and records the latency of every edit it
// receives, until ctx is cancelled.
func reader(ctx context.Context, cfg config, st *stats, id string) {
	conn, err := dial(ctx, cfg.baseURL, id)
	if err != nil {
		st.connectErrs.Add(1)
		return
	}
	st.connected.Add(1)
	defer conn.CloseNow()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // ctx was cancelled, or the server closed us
		}
		if t, ok := sentAt(data); ok {
			st.record(time.Since(t))
		}
	}
}
// writer connects to room id and sends one edit every cfg.interval until
// ctx is cancelled.
func writer(ctx context.Context, cfg config, st *stats, id string) {
	conn, err := dial(ctx, cfg.baseURL, id)
	if err != nil {
		st.connectErrs.Add(1)
		return
	}
	st.connected.Add(1)
	defer conn.CloseNow()

	// The first frame is the room's initial content. Read it here, because
	// CloseRead below closes the connection if any data frame arrives.
	if _, _, err := conn.Read(ctx); err != nil {
		return
	}
	ctx = conn.CloseRead(ctx)

	sleep(ctx, jitter(cfg.interval))
	t := time.NewTicker(cfg.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := conn.Write(ctx, websocket.MessageText, payload(cfg.size))
			if err != nil {
				st.sendErrs.Add(1)
				return
			}
			st.sent.Add(1)
		}
	}
}

func run(ctx context.Context, cfg config) (*stats, error) {
	st := &stats{}

	// Create the rooms first, one request at a time.
	ids := make([]string, 0, cfg.rooms)
	for range cfg.rooms {
		id, err := createRoom(ctx, cfg.baseURL)
		if err != nil {
			return nil, fmt.Errorf("create room: %w", err)
		}
		ids = append(ids, id)
	}
	log.Printf("created %d rooms, starting %d clients", len(ids), cfg.rooms*cfg.clients)

	// Every client stops when this context ends.
	ctx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

		var wg sync.WaitGroup
	for _, id := range ids {
		for range cfg.clients - 1 {
			wg.Go(func() { reader(ctx, cfg, st, id) })
		}
		wg.Go(func() { writer(ctx, cfg, st, id) })
	}
	wg.Wait()

	return st, nil
}

func report(cfg config, st *stats) {
	st.mu.Lock()
	lat := slices.Clone(st.latencies)
	st.mu.Unlock()
	slices.Sort(lat)

	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[int(float64(len(lat)-1)*p)].Round(time.Microsecond)
	}

	sent := st.sent.Load()
	expected := sent * int64(cfg.clients-1)
	delivered := 0.0
	if expected > 0 {
		delivered = 100 * float64(st.received.Load()) / float64(expected)
	}

	fmt.Println()
	fmt.Printf("clients     %d connected of %d (%d failed)\n",
		st.connected.Load(), cfg.rooms*cfg.clients, st.connectErrs.Load())
	fmt.Printf("edits       %d sent (%d bytes each), %d send errors\n",
		sent, cfg.size, st.sendErrs.Load())
	fmt.Printf("deliveries  %d of %d expected (%.1f%%)\n",
		st.received.Load(), expected, delivered)
	fmt.Printf("latency     p50 %v   p95 %v   p99 %v   max %v\n",
		pct(0.50), pct(0.95), pct(0.99), pct(1))
}

func main() {
	var cfg config
	flag.StringVar(&cfg.baseURL, "url", "http://localhost:8071", "base URL of a TEST instance")
	flag.IntVar(&cfg.rooms, "rooms", 50, "number of rooms")
	flag.IntVar(&cfg.clients, "clients", 2, "clients per room (one writes, the rest read)")
	flag.DurationVar(&cfg.interval, "interval", time.Second, "time between edits in each room")
	flag.IntVar(&cfg.size, "size", 2048, "bytes per edit")
	flag.DurationVar(&cfg.duration, "duration", 30*time.Second, "how long to run")
	flag.Parse()

	cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
	if cfg.rooms < 1 || cfg.clients < 2 {
		log.Fatal("need at least 1 room and 2 clients per room")
	}

	// Ctrl+C stops the run early and still prints the report.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	st, err := run(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	report(cfg, st)
}
