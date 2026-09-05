package connection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/model"
)

// A file written before broker kinds existed has no schemaVersion, keeps the
// endpoint under nameServer, and carries the two ACL keys as top-level ENC:
// strings. Loading one must produce a working profile without asking anyone to
// re-enter a credential.
func TestLoadMigratesThePreKindFormat(t *testing.T) {
	ensureTestCrypto(t)
	encryptedAccessKey, err := crypto.Encrypt("legacy-ak", model.SecretAccessKey)
	if err != nil {
		t.Fatal(err)
	}
	encryptedSecretKey, err := crypto.Encrypt("legacy-sk", model.SecretSecretKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "connections.json")
	legacy := `{"connections":[{"id":3,"name":"prod","group":"staging",` +
		`"nameServer":"ns-a:9876;ns-b:9876","timeoutSec":8,"enableACL":true,` +
		`"accessKey":"` + encryptedAccessKey + `","secretKey":"` + encryptedSecretKey + `",` +
		`"isDefault":true,"remark":"note"}]}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	service := New(path, fakeSettings{connectTimeout: 3 * time.Second}, noopRuntime{}, addressedEndpoints{})

	profiles := service.GetConnections()
	if len(profiles) != 1 {
		t.Fatalf("loaded %d profiles, want 1", len(profiles))
	}
	profile := profiles[0]
	if profile.Kind != model.KindRocketMQ {
		t.Errorf("kind = %q, want rocketmq for a file that predates kinds", profile.Kind)
	}
	if profile.Endpoints != "ns-a:9876;ns-b:9876" {
		t.Errorf("endpoints = %q, want the nameServer value", profile.Endpoints)
	}
	if !profile.ACLEnabled() {
		t.Error("enableACL did not become the acl mechanism")
	}
	if profile.Secret(model.SecretAccessKey) != "legacy-ak" {
		t.Errorf("accessKey = %q, want the decrypted legacy value", profile.Secret(model.SecretAccessKey))
	}
	if profile.Secret(model.SecretSecretKey) != "legacy-sk" {
		t.Errorf("secretKey = %q, want the decrypted legacy value", profile.Secret(model.SecretSecretKey))
	}
	if profile.Name != "prod" || profile.Group != "staging" || profile.Remark != "note" {
		t.Errorf("unrelated fields changed: %#v", profile)
	}
}

// Saving after a migration writes the new shape, and the credentials must go
// back encrypted rather than being written out in the clear.
func TestSaveWritesTheKindSchemaWithEncryptedSecrets(t *testing.T) {
	ensureTestCrypto(t)
	path := filepath.Join(t.TempDir(), "connections.json")
	service := New(path, fakeSettings{connectTimeout: 3 * time.Second}, noopRuntime{}, addressedEndpoints{})

	if _, err := service.AddConnection(profileOf("prod", "", "ns:9876", 5, true, "plain-ak", "plain-sk", "")); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var written struct {
		SchemaVersion int `json:"schemaVersion"`
		Connections   []struct {
			Kind      model.MQKind      `json:"kind"`
			Endpoints string            `json:"endpoints"`
			Auth      model.AuthConfig  `json:"auth"`
			Secrets   map[string]string `json:"secrets"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if written.SchemaVersion != currentSchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", written.SchemaVersion, currentSchemaVersion)
	}
	if len(written.Connections) != 1 {
		t.Fatalf("wrote %d connections, want 1", len(written.Connections))
	}
	record := written.Connections[0]
	if record.Kind != model.KindRocketMQ || record.Endpoints != "ns:9876" {
		t.Errorf("kind/endpoints not written: %#v", record)
	}
	if record.Auth.Mechanism != model.AuthACL {
		t.Errorf("auth mechanism = %q, want acl", record.Auth.Mechanism)
	}
	if record.Secrets[model.SecretAccessKey] == "plain-ak" {
		t.Error("access key was written in the clear")
	}
	if record.Secrets[model.SecretAccessKey] == "" {
		t.Error("access key was dropped instead of encrypted")
	}
}
