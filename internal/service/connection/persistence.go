package connection

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/storage/atomicfile"
)

// currentSchemaVersion marks the kind-based profile format. A file without it
// predates broker kinds and is migrated on load.
const currentSchemaVersion = 1

type store struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Connections   []*model.ConnectionRecord `json:"connections"`
}

func (s *Service) loadConnectionsFromFile() error {
	data, err := os.ReadFile(s.dataFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var persisted store
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	connections := make([]*model.ConnectionProfile, 0, len(persisted.Connections))
	for _, record := range persisted.Connections {
		if record == nil {
			continue
		}
		current := record.Profile()
		/*
		 * Every credential, because every one of them was encrypted on the way
		 * out. Decrypting only RocketMQ's access key pair left a RabbitMQ
		 * password loaded as its own ciphertext, which the driver then sent to
		 * the broker as the password.
		 *
		 * A value with no marker is passed through: a file written before
		 * encryption existed, or one edited by hand, is still readable.
		 */
		for key, stored := range current.Secrets {
			if stored == "" || !crypto.IsEncrypted(stored) {
				continue
			}
			plain, decryptErr := crypto.Decrypt(stored, key)
			if decryptErr != nil {
				return fmt.Errorf("failed to decrypt %s for connection %q: %w", key, current.Name, decryptErr)
			}
			current.SetSecret(key, plain)
		}
		connections = append(connections, current)
	}

	s.connections, s.nextID = buildConnectionState(connections)
	return nil
}

// buildConnectionState applies the compatibility rules used when loading saved profiles.
func buildConnectionState(connections []*model.ConnectionProfile) (map[int]*model.ConnectionProfile, int) {
	loaded := make(map[int]*model.ConnectionProfile, len(connections))
	nextID := 1
	hasDefault := false
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		current := *connection
		if current.ID <= 0 {
			current.ID = nextID
		}
		if current.ID >= nextID {
			nextID = current.ID + 1
		}
		if _, exists := loaded[current.ID]; exists {
			current.ID = nextID
			nextID++
		}

		current.Group = normalizeConnectionGroup(current.Group)
		current.Status = model.StatusOffline
		current.LastCheck = "-"
		mechanism := current.Auth.Mechanism
		enabled, accessKey, secretKey, err := normalizeACLConfig(current.ACLEnabled(), current.Secret(model.SecretAccessKey), current.Secret(model.SecretSecretKey))
		if err != nil {
			enabled, accessKey, secretKey = false, "", ""
		}
		current.SetACL(enabled, accessKey, secretKey)
		// SetACL files a non-ACL connection as anonymous, which is what "no
		// ACL" means for the one family whose only mechanism is ACL. Another
		// family's mechanism has to survive the load, or every restart turned
		// a RabbitMQ connection into an anonymous one.
		if !enabled && mechanism != model.AuthACL {
			current.Auth.Mechanism = mechanism
		}

		if current.IsDefault {
			if hasDefault {
				current.IsDefault = false
			} else {
				hasDefault = true
			}
		}
		copy := current
		loaded[current.ID] = &copy
	}

	if len(loaded) > 0 && !hasDefault {
		ids := sortedConnectionIDs(loaded)
		loaded[ids[0]].IsDefault = true
	}
	return loaded, nextID
}

func sortedConnectionIDs(connections map[int]*model.ConnectionProfile) []int {
	ids := make([]int, 0, len(connections))
	for id := range connections {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func copyConnectionsSorted(connections map[int]*model.ConnectionProfile) []*model.ConnectionProfile {
	result := make([]*model.ConnectionProfile, 0, len(connections))
	for _, id := range sortedConnectionIDs(connections) {
		connection := connections[id]
		if connection == nil {
			continue
		}
		copy := *connection
		result = append(result, &copy)
	}
	return result
}

func encodeConnectionsForDisk(connections []*model.ConnectionProfile) ([]byte, error) {
	persisted := store{
		SchemaVersion: currentSchemaVersion,
		Connections:   make([]*model.ConnectionRecord, 0, len(connections)),
	}
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		record := model.NewConnectionRecord(connection)
		for key, value := range record.Secrets {
			encrypted, err := crypto.Encrypt(value, key)
			if err != nil {
				return nil, fmt.Errorf("failed to encrypt %s: %w", key, err)
			}
			record.Secrets[key] = encrypted
		}
		persisted.Connections = append(persisted.Connections, record)
	}
	return json.MarshalIndent(persisted, "", "  ")
}

// saveConnectionsLocked persists the current state. The caller must hold s.mu for writing.
func (s *Service) saveConnectionsLocked() error {
	data, err := encodeConnectionsForDisk(copyConnectionsSorted(s.connections))
	if err != nil {
		return err
	}
	return atomicfile.Write(s.dataFilePath, data)
}

func (s *Service) prepareReplacement(connections []*model.ConnectionProfile) ([]*model.ConnectionProfile, error) {
	prepared := make([]*model.ConnectionProfile, 0, len(connections))
	for _, connection := range connections {
		if connection == nil {
			continue
		}
		current := *connection
		name, nameServer, err := s.validateConnectionFields(current.Kind, current.Name, current.Endpoints, current.TimeoutSec)
		if err != nil {
			return nil, fmt.Errorf("invalid connection %q: %w", current.Name, err)
		}
		enabled, accessKey, secretKey, err := normalizeACLConfig(current.ACLEnabled(), current.Secret(model.SecretAccessKey), current.Secret(model.SecretSecretKey))
		if err != nil {
			return nil, fmt.Errorf("invalid ACL configuration for connection %q: %w", name, err)
		}
		current.Name = name
		current.Endpoints = nameServer
		current.Group = normalizeConnectionGroup(current.Group)
		current.SetACL(enabled, accessKey, secretKey)
		current.Status = model.StatusOffline
		current.LastCheck = "-"
		prepared = append(prepared, &current)
	}
	normalized, _ := buildConnectionState(prepared)
	return copyConnectionsSorted(normalized), nil
}

// ValidateConnections verifies that profiles can be normalized and encoded
// without changing persisted or runtime connection state.
func (s *Service) ValidateConnections(connections []*model.ConnectionProfile) error {
	prepared, err := s.prepareReplacement(connections)
	if err != nil {
		return err
	}
	_, err = encodeConnectionsForDisk(prepared)
	return err
}

// ReplaceConnections validates and atomically replaces all persisted connection profiles.
func (s *Service) ReplaceConnections(connections []*model.ConnectionProfile) error {
	prepared, err := s.prepareReplacement(connections)
	if err != nil {
		return err
	}
	data, err := encodeConnectionsForDisk(prepared)
	if err != nil {
		return err
	}

	defer s.notifyChanged()
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()

	s.mu.Lock()
	plan := s.reloadPlanLocked()
	if err := atomicfile.Write(s.dataFilePath, data); err != nil {
		s.mu.Unlock()
		return err
	}
	s.connections, s.nextID = buildConnectionState(prepared)
	plan = s.finalizeReloadPlanLocked(plan)
	s.mu.Unlock()

	return s.restoreRuntimeLocked(plan)
}
