package store

import (
	"sync"
	"time"
	"context"
	"bytes"
)

type Memory struct {
	mu     sync.Mutex
	pastes map[string]Paste
	now    func() time.Time // injectable for tests
}

func NewMemory() *Memory {
	return &Memory{pastes: make(map[string]Paste), now: time.Now}
}

func (m *Memory) Put(ctx context.Context, p Paste) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p.Content = bytes.Clone(p.Content) // own a copy; the caller may mutate theirs
	m.pastes[p.ID] = p
	return nil
}

func (m *Memory) Get(ctx context.Context, id string) (Paste, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.pastes[id]
	if !ok || p.ExpiresAt.Before(m.now()) {
		return Paste{}, ErrNotFound
	}
	p.Content = bytes.Clone(p.Content)
	return p, nil
}

func (m *Memory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.pastes[id]; !ok {
		return ErrNotFound
	}
	delete(m.pastes, id)
	return nil
}

func (m *Memory) Expire(ctx context.Context, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int
	for id, p := range m.pastes { 
		if p.ExpiresAt.Before(now) {
			delete(m.pastes, id);
			n++;
		} 
	}
	return n, nil
}