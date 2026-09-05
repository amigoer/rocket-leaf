package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/model"
	connectionservice "github.com/amigoer/mq-studio/internal/service/connection"
	settingsservice "github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

type failAfterReplaceConnections struct {
	*connectionservice.Service
	failNext bool
}

func (f *failAfterReplaceConnections) ReplaceConnections(connections []*model.ConnectionProfile) error {
	if err := f.Service.ReplaceConnections(connections); err != nil {
		return err
	}
	if f.failNext {
		f.failNext = false
		return errors.New("injected failure after replacement")
	}
	return nil
}

func TestRealServicesConfigurationWorkflow(t *testing.T) {
	directory := t.TempDir()
	paths := layout.In(directory)
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settings := settingsservice.New(paths.SettingsFile)
	connections := connectionservice.New(paths.ConnectionsFile, settings, noopClientRuntime{}, addressedEndpoints{})
	configuration := New(paths, settings, connections)

	t.Run("portable export and import", func(t *testing.T) {
		nextSettings := *model.DefaultSettings()
		nextSettings.Theme = "dark"
		nextSettings.Language = "en"
		nextSettings.GlobalAccessKey = "global-ak"
		nextSettings.GlobalSecretKey = "global-sk"
		if _, err := configuration.UpdateSettings(nextSettings); err != nil {
			t.Fatal(err)
		}
		if err := connections.ReplaceConnections([]*model.ConnectionProfile{{
			ID:         4,
			Name:       "production",
			Group:      "production",
			Endpoints:  "ns-a:9876;ns-b:9876",
			TimeoutSec: 8,
			Auth:       model.AuthConfig{Mechanism: model.AuthACL},
			Secrets: map[string]string{
				model.SecretAccessKey: "connection-ak",
				model.SecretSecretKey: "connection-sk",
			},
			IsDefault: true,
		}}); err != nil {
			t.Fatal(err)
		}

		exported, err := configuration.ExportAllConfig()
		if err != nil {
			t.Fatal(err)
		}
		var payload exportPayload
		if err := json.Unmarshal([]byte(exported), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Version != currentExportVersion || !payload.ContainsSecrets {
			t.Fatalf("unexpected export metadata: %#v", payload)
		}
		if payload.Settings.GlobalAccessKey != "global-ak" || payload.Settings.GlobalSecretKey != "global-sk" {
			t.Fatalf("global credentials were not exported portably: %#v", payload.Settings)
		}
		if len(payload.Connections.Connections) != 1 ||
			payload.Connections.Connections[0].Secret(model.SecretAccessKey) != "connection-ak" ||
			payload.Connections.Connections[0].Secret(model.SecretSecretKey) != "connection-sk" {
			t.Fatalf("connection credentials were not exported portably: %#v", payload.Connections)
		}

		settingsBeforeReservedExport, err := os.ReadFile(paths.SettingsFile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := configuration.ExportAllConfigToFile(paths.SettingsFile); err == nil {
			t.Fatal("export should reject the settings persistence path")
		}
		settingsAfterReservedExport, err := os.ReadFile(paths.SettingsFile)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(settingsBeforeReservedExport, settingsAfterReservedExport) {
			t.Fatal("reserved-path export changed the settings file")
		}

		if _, err := configuration.ResetSettings(); err != nil {
			t.Fatal(err)
		}
		if err := connections.ReplaceConnections(nil); err != nil {
			t.Fatal(err)
		}
		if err := configuration.ImportAllConfig(exported); err != nil {
			t.Fatal(err)
		}
		if got := configuration.GetSettings(); got.Theme != "dark" || got.GlobalAccessKey != "global-ak" || got.GlobalSecretKey != "global-sk" {
			t.Fatalf("settings were not restored: %#v", got)
		}
		gotConnections := connections.GetConnections()
		if len(gotConnections) != 1 || gotConnections[0].Secret(model.SecretAccessKey) != "connection-ak" || gotConnections[0].Secret(model.SecretSecretKey) != "connection-sk" {
			t.Fatalf("connections were not restored: %#v", gotConnections)
		}
		assertNoPlaintextCredentials(t, paths.SettingsFile, "global-ak", "global-sk")
		assertNoPlaintextCredentials(t, paths.ConnectionsFile, "connection-ak", "connection-sk")
	})

	t.Run("version 2 keeps ENC prefix as plaintext", func(t *testing.T) {
		raw := marshalJSON(t, map[string]any{
			"version": currentExportVersion,
			"connections": connectionStore{Connections: []*model.ConnectionRecord{{ConnectionProfile: model.ConnectionProfile{
				ID:         1,
				Name:       "literal-prefix",
				Endpoints:  "ns:9876",
				TimeoutSec: 5,
				Auth:       model.AuthConfig{Mechanism: model.AuthACL},
			}, Secrets: map[string]string{
				model.SecretAccessKey: "ENC:literal-ak",
				model.SecretSecretKey: "ENC:literal-sk",
			}}}},
		})
		if err := configuration.ImportAllConfig(raw); err != nil {
			t.Fatal(err)
		}
		got := connections.GetConnections()
		if len(got) != 1 || got[0].Secret(model.SecretAccessKey) != "ENC:literal-ak" || got[0].Secret(model.SecretSecretKey) != "ENC:literal-sk" {
			t.Fatalf("version 2 credentials changed: %#v", got)
		}
		assertNoPlaintextCredentials(t, paths.ConnectionsFile, "ENC:literal-ak", "ENC:literal-sk")
	})

	t.Run("version 1 decrypts embedded credentials", func(t *testing.T) {
		encryptedAccessKey, err := crypto.Encrypt("legacy-ak", "accessKey")
		if err != nil {
			t.Fatal(err)
		}
		encryptedSecretKey, err := crypto.Encrypt("legacy-sk", "secretKey")
		if err != nil {
			t.Fatal(err)
		}
		raw := marshalJSON(t, map[string]any{
			"version": 1,
			"connections": connectionStore{Connections: []*model.ConnectionRecord{{ConnectionProfile: model.ConnectionProfile{
				ID:         2,
				Name:       "legacy",
				Endpoints:  "legacy-ns:9876",
				TimeoutSec: 5,
				Auth:       model.AuthConfig{Mechanism: model.AuthACL},
			}, LegacyAccessKey: encryptedAccessKey, LegacySecretKey: encryptedSecretKey}}},
		})
		if err := configuration.ImportAllConfig(raw); err != nil {
			t.Fatal(err)
		}
		got := connections.GetConnections()
		if len(got) != 1 || got[0].Secret(model.SecretAccessKey) != "legacy-ak" || got[0].Secret(model.SecretSecretKey) != "legacy-sk" {
			t.Fatalf("version 1 credentials were not decrypted: %#v", got)
		}
		assertNoPlaintextCredentials(t, paths.ConnectionsFile, "legacy-ak", "legacy-sk")
	})

	t.Run("failed connection apply rolls back both domains", func(t *testing.T) {
		baselineSettings := *model.DefaultSettings()
		baselineSettings.Theme = "light"
		baselineSettings.GlobalAccessKey = "baseline-global-ak"
		baselineSettings.GlobalSecretKey = "baseline-global-sk"
		if _, err := configuration.UpdateSettings(baselineSettings); err != nil {
			t.Fatal(err)
		}
		baselineConnections := []*model.ConnectionProfile{{
			ID:         11,
			Name:       "baseline",
			Endpoints:  "baseline-ns:9876",
			TimeoutSec: 5,
			Auth:       model.AuthConfig{Mechanism: model.AuthACL},
			Secrets: map[string]string{
				model.SecretAccessKey: "baseline-ak",
				model.SecretSecretKey: "baseline-sk",
			},
		}}
		if err := connections.ReplaceConnections(baselineConnections); err != nil {
			t.Fatal(err)
		}

		failingConnections := &failAfterReplaceConnections{Service: connections, failNext: true}
		coordinator := New(paths, settings, failingConnections)
		updatedSettings := *model.DefaultSettings()
		updatedSettings.Theme = "dark"
		raw := marshalJSON(t, map[string]any{
			"version":  currentExportVersion,
			"settings": updatedSettings,
			"connections": connectionStore{Connections: []*model.ConnectionRecord{{ConnectionProfile: model.ConnectionProfile{
				ID:         12,
				Name:       "replacement",
				Endpoints:  "replacement-ns:9876",
				TimeoutSec: 5,
			}}}},
		})
		err := coordinator.ImportAllConfig(raw)
		if err == nil || !strings.Contains(err.Error(), "已回滚") {
			t.Fatalf("error = %v, want successful rollback", err)
		}
		if got := settings.GetSettings(); got.Theme != "light" || got.GlobalAccessKey != "baseline-global-ak" || got.GlobalSecretKey != "baseline-global-sk" {
			t.Fatalf("settings rollback failed: %#v", got)
		}
		gotConnections := connections.GetConnections()
		if len(gotConnections) != 1 || gotConnections[0].Name != "baseline" || gotConnections[0].Secret(model.SecretAccessKey) != "baseline-ak" {
			t.Fatalf("connection rollback failed: %#v", gotConnections)
		}
		connectionData, err := os.ReadFile(paths.ConnectionsFile)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(connectionData, []byte("replacement")) {
			t.Fatal("rolled-back connection remained on disk")
		}
		assertNoPlaintextCredentials(t, paths.SettingsFile, "baseline-global-ak", "baseline-global-sk")
		assertNoPlaintextCredentials(t, paths.ConnectionsFile, "baseline-ak", "baseline-sk")
	})
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertNoPlaintextCredentials(t *testing.T, path string, credentials ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range credentials {
		if bytes.Contains(data, []byte(credential)) {
			t.Fatalf("%s contains plaintext credential %q", path, credential)
		}
	}
}

// noopClientRuntime stands in for the client registry: this test exercises
// config import and export, not connection lifecycle.
type noopClientRuntime struct{}

func (noopClientRuntime) Connect(model.ConnectionProfile) error { return nil }
func (noopClientRuntime) HasClient(int) bool                    { return false }
func (noopClientRuntime) Remove(int)                            {}
func (noopClientRuntime) Test(model.ConnectionProfile) error    { return nil }
func (noopClientRuntime) CloseAll()                             {}

// addressedEndpoints stands in for the driver-backed policy: every profile in
// this test names a family that is dialled by address.
type addressedEndpoints struct{}

func (addressedEndpoints) RequiresEndpoints(model.MQKind) bool { return true }
