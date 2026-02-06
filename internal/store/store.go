// Package store provides a thread-safe, file-backed key-value store for links.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"
)

// Store manages the thread-safe storage and persistence of links.
type Store struct {
	mu     sync.RWMutex
	links  map[string]string
	dbFile string
	logger *slog.Logger
}

// New creates a Store and loads existing links from disk.
// If the database file does not exist, an empty store is returned.
func New(dbFile string, logger *slog.Logger) (*Store, error) {
	s := &Store{
		links:  make(map[string]string),
		dbFile: dbFile,
		logger: logger,
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("load links: %w", err)
	}
	return s, nil
}

// Get returns the URL associated with key and whether it exists.
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.links[key]
	return v, ok
}

// Set creates or overwrites a link and persists to disk.
func (s *Store) Set(key, url string) error {
	s.mu.Lock()
	s.links[key] = url
	s.mu.Unlock()
	return s.save()
}

// Delete removes a link by key. Returns false if the key did not exist.
func (s *Store) Delete(key string) (bool, error) {
	s.mu.Lock()
	if _, ok := s.links[key]; !ok {
		s.mu.Unlock()
		return false, nil
	}
	delete(s.links, key)
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return true, err
	}
	return true, nil
}

// Update changes the URL of an existing link. Returns false if the key does not exist.
func (s *Store) Update(key, newURL string) (bool, error) {
	s.mu.Lock()
	if _, ok := s.links[key]; !ok {
		s.mu.Unlock()
		return false, nil
	}
	s.links[key] = newURL
	s.mu.Unlock()

	if err := s.save(); err != nil {
		return true, err
	}
	return true, nil
}

// All returns a snapshot copy of all links.
func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := make(map[string]string, len(s.links))
	for k, v := range s.links {
		snapshot[k] = v
	}
	return snapshot
}

// Save persists all links to disk. Exposed for shutdown hooks.
func (s *Store) Save() error {
	return s.save()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.dbFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.logger.Info("no existing database file, starting fresh", "file", s.dbFile)
			return nil
		}
		return fmt.Errorf("read %s: %w", s.dbFile, err)
	}
	if len(data) == 0 {
		return nil
	}

	var links map[string]string
	if err := json.Unmarshal(data, &links); err != nil {
		return fmt.Errorf("decode %s: %w", s.dbFile, err)
	}
	s.links = links
	s.logger.Info("loaded links", "count", len(links))
	return nil
}

// save writes all links to a temporary file, then atomically renames it
// to the database file to avoid corruption on crash.
func (s *Store) save() error {
	s.mu.RLock()
	snapshot := make(map[string]string, len(s.links))
	for k, v := range s.links {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode links: %w", err)
	}

	tmp := s.dbFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.dbFile); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, s.dbFile, err)
	}
	return nil
}
