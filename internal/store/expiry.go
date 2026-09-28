package store

import (
	"context"
	"log"
	"time"
)

// RunExpiry deletes expired pastes every interval until ctx is cancelled.
// Run it with `go store.RunExpiry(ctx, st, time.Minute)`.
//
// TODO(you): implement with a time.Ticker and a select over two channels:

// This is the same select shape as room.run, with a clock instead of
// clients. ctx.Done() is a channel that closes when the context is
// cancelled; receiving from a closed channel returns immediately, which
// is how every goroutine in the program learns it is time to stop.
func RunExpiry(ctx context.Context, s Store, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop() // tickers leak if not stopped
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C: // t.C delivers the current time each tick
			n, err := s.Expire(ctx, now)
			if err != nil {
				log.Printf("expiry: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("expiry: removed %d paste(s)", n)
			}
		}
	}
}
