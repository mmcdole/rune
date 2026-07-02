package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The durable store (lua.Host implementation): key→JSON, backed by
// <config>/store.json. Reads come from the in-memory map (loaded once
// in New); every write rewrites the file atomically via temp+rename,
// so a crash mid-write cannot corrupt it. Writes happen at user-
// command frequency, so write-through needs no debouncing. All
// methods run on the session goroutine - no locking.

// loadStore reads store.json into memory. Called from New. A corrupt
// file is preserved as store.json.bak and reported at boot - user
// data is never silently discarded.
func (s *Session) loadStore() {
	s.store = make(map[string]json.RawMessage)
	s.storePath = filepath.Join(s.config.ConfigDir, "store.json")

	data, err := os.ReadFile(s.storePath)
	if err != nil {
		if !os.IsNotExist(err) {
			s.storeLoadErr = fmt.Errorf("reading %s: %w (durable store disabled this session)", s.storePath, err)
		}
		return
	}
	if err := json.Unmarshal(data, &s.store); err != nil {
		backup := s.storePath + ".bak"
		if renameErr := os.Rename(s.storePath, backup); renameErr != nil {
			backup = "(backup failed: " + renameErr.Error() + ")"
		}
		s.storeLoadErr = fmt.Errorf("%s is corrupt (%v); preserved as %s, starting empty", s.storePath, err, backup)
		s.store = make(map[string]json.RawMessage)
	}
}

// saveStore writes the whole store atomically.
func (s *Session) saveStore() error {
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.store, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "store-*.json.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.storePath)
}

// StoreSet implements lua.Host.
func (s *Session) StoreSet(key, rawJSON string) error {
	s.store[key] = json.RawMessage(rawJSON)
	return s.saveStore()
}

// StoreGet implements lua.Host.
func (s *Session) StoreGet(key string) (string, bool) {
	raw, ok := s.store[key]
	return string(raw), ok
}

// StoreDelete implements lua.Host.
func (s *Session) StoreDelete(key string) error {
	if _, ok := s.store[key]; !ok {
		return nil
	}
	delete(s.store, key)
	return s.saveStore()
}
