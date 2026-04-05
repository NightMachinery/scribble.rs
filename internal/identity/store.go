package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/uuid/v5"
)

type nameStore struct {
	mutex sync.RWMutex
	path  string
	names map[string]string
}

var globalStore = &nameStore{
	names: make(map[string]string),
}

// Init configures the durable display-name store and loads any existing data.
func Init(path string) error {
	store := &nameStore{
		path:  path,
		names: make(map[string]string),
	}

	if err := store.load(); err != nil {
		return err
	}

	globalStore = store
	return nil
}

// GetName returns the persisted display name for the supplied client ID.
func GetName(clientID uuid.UUID) (string, bool) {
	if clientID == uuid.Nil {
		return "", false
	}

	globalStore.mutex.RLock()
	defer globalStore.mutex.RUnlock()

	name, ok := globalStore.names[clientID.String()]
	return name, ok
}

// SetName persists the display name for the supplied client ID.
func SetName(clientID uuid.UUID, name string) error {
	if clientID == uuid.Nil || name == "" {
		return nil
	}

	return globalStore.setName(clientID.String(), name)
}

func (store *nameStore) load() error {
	if store.path == "" {
		return nil
	}

	bytes, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("error reading player identity store: %w", err)
	}

	if len(bytes) == 0 {
		return nil
	}

	if err := json.Unmarshal(bytes, &store.names); err != nil {
		return fmt.Errorf("error unmarshalling player identity store: %w", err)
	}

	return nil
}

func (store *nameStore) setName(clientID, name string) error {
	store.mutex.Lock()
	store.names[clientID] = name
	snapshot := make(map[string]string, len(store.names))
	for key, value := range store.names {
		snapshot[key] = value
	}
	path := store.path
	store.mutex.Unlock()

	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("error creating player identity store directory: %w", err)
	}

	bytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling player identity store: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(path), "player-identities-*.tmp")
	if err != nil {
		return fmt.Errorf("error creating temp player identity store file: %w", err)
	}

	tempName := tempFile.Name()
	defer os.Remove(tempName)

	if _, err := tempFile.Write(bytes); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("error writing temp player identity store file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("error closing temp player identity store file: %w", err)
	}

	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("error replacing player identity store file: %w", err)
	}

	return nil
}
