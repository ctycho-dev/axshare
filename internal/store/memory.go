package store

import (
	"bytes"
	"context"
	"sync"
	"time"
)

type Memory struct {
	mu    sync.Mutex
	rooms map[string]Room
	now   func() time.Time // injectable for tests
}

func NewMemory() *Memory {
	return &Memory{rooms: make(map[string]Room), now: time.Now}
}

func (m *Memory) Put(ctx context.Context, p Room) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ext is set on create only; an existing room keeps the one it has.
	if old, ok := m.rooms[p.ID]; ok {
		p.Ext = old.Ext
	} else if p.Ext == "" {
		p.Ext = DefaultExt
	}
	p.Content = bytes.Clone(p.Content) // own a copy; the caller may mutate theirs
	m.rooms[p.ID] = p
	return nil
}

func (m *Memory) SetExt(ctx context.Context, id, ext string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.rooms[id]
	if !ok || p.ExpiresAt.Before(m.now()) {
		return ErrNotFound
	}
	p.Ext = ext
	m.rooms[id] = p
	return nil
}

func (m *Memory) Get(ctx context.Context, id string) (Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.rooms[id]
	if !ok || p.ExpiresAt.Before(m.now()) {
		return Room{}, ErrNotFound
	}
	p.Content = bytes.Clone(p.Content)
	return p, nil
}

func (m *Memory) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.rooms[id]; !ok {
		return ErrNotFound
	}
	delete(m.rooms, id)
	return nil
}

func (m *Memory) Expire(ctx context.Context, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int
	for id, p := range m.rooms {
		if p.ExpiresAt.Before(now) {
			delete(m.rooms, id)
			n++
		}
	}
	return n, nil
}
