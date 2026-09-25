package library

import (
	"log"
	"path/filepath"
	"sync"
	"time"
)

// Manager owns the current scan and keeps it fresh.
type Manager struct {
	root   string
	prober *Prober

	mu  sync.RWMutex
	lib *Library
}

func NewManager(root string) *Manager {
	return &Manager{root: root, prober: NewProber()}
}

// Scan rescans the shelf and swaps the snapshot in. Duration and resolution
// arrive afterwards in the background: the first page paint should not wait
// on ffprobe.
func (m *Manager) Scan() (int, error) {
	lib, err := Scan(m.root)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	m.lib = lib
	m.mu.Unlock()
	log.Printf("library: %d videos, %d folders", len(lib.Items), len(lib.Folders))

	go func() {
		probe := make([]Item, len(lib.Items))
		copy(probe, lib.Items)
		m.prober.Enrich(probe, m.root)
		lib.Items = probe
		m.mu.Lock()
		// Keep the newer scan if one happened while probing.
		if m.lib != nil && m.lib.ScannedAt.Equal(lib.ScannedAt) {
			m.lib = lib
		}
		m.mu.Unlock()
	}()

	return len(lib.Items), nil
}

// Snapshot returns the shelf as it stands. The caller must not mutate it.
func (m *Manager) Snapshot() *Library {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lib
}

// Start keeps the shelf fresh until the channel closes.
func (m *Manager) Start(every time.Duration) {
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := m.Scan(); err != nil {
				log.Printf("library: rescan failed: %v", err)
			}
		}
	}()
}

// PosterFor finds cover art next to the video with the given id.
func (m *Manager) PosterFor(item Item) string {
	return posterFor(filepath.Join(m.root, filepath.FromSlash(item.RelPath)))
}

// PathOf resolves an item to a file on disk.
func (m *Manager) PathOf(item Item) string {
	return filepath.Join(m.root, filepath.FromSlash(item.RelPath))
}
