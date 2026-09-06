package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The credential trap this family walks straight past, pinned offline.
 *
 * accessKey and secretKey are RocketMQ's ACL pair and are reserved in three
 * places: they skip applyCredentials' generic loop, they are written only
 * through SetACL, and resolveACLCredentials fills them from the global
 * settings for any profile that named no mechanism of its own. A family
 * reusing those names would have its credential cleared on save and
 * RocketMQ's global pair stamped on at dial time - and nothing else would go
 * red, because the profile would still save, still reload and still look
 * complete.
 *
 * So the settings here hold a global pair, which is the state in which it
 * would bite, and the assertion is the whole round trip: a Pub/Sub profile
 * with no address saves, comes back off a second service reading the same file
 * with its own key intact, keeps its own mechanism, and is resolved for
 * dialling without the reserved names being filled in behind it.
 *
 * The key is checked byte for byte rather than merely present. It is a JSON
 * document rather than a short string, and the private key inside it is
 * line-sensitive: a store that trimmed or re-wrapped it would produce a
 * credential that saves, reloads, looks right and cannot sign.
 *
 * It lives here rather than in internal/service/connection because this is
 * where the two halves meet: the composition root binds the endpoint policy to
 * the driver's own descriptor, and it is that descriptor - with no endpoint
 * field on it - that makes an empty address savable at all.
 */
func TestGooglePubSubCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindGooglePubSub); !ok {
		driver.Register(pubsubdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}

	settingsService := settings.New(paths.SettingsFile)
	stored := settingsService.GetSettings()
	stored.GlobalAccessKey = "rocketmq-global-ak"
	stored.GlobalSecretKey = "rocketmq-global-sk"
	if _, err := settingsService.UpdateSettings(*stored); err != nil {
		t.Fatalf("store the global acl pair: %v", err)
	}

	connections := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())

	// A service account key as a user would paste one: real JSON, several
	// lines, and an embedded private key whose newlines are load-bearing.
	const key = "{\n  \"type\": \"service_account\",\n  \"project_id\": \"orders-prod\",\n" +
		"  \"private_key\": \"-----BEGIN PRIVATE KEY-----\\nMIIB\\n-----END PRIVATE KEY-----\\n\"\n}"

	input := model.ConnectionProfile{
		Name:       "pub/sub with a global acl pair configured",
		Kind:       model.KindGooglePubSub,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{pubsubdriver.OptionProjectID: "orders-prod"},
	}
	input.SetSecret(pubsubdriver.SecretCredentialsJSON, key)

	added, err := connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if added.Endpoints != "" {
		t.Fatalf("the saved profile carries an address %q; this family has none", added.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())
	reloaded, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}

	if got := reloaded.Secret(pubsubdriver.SecretCredentialsJSON); got != key {
		t.Errorf("the service account key did not survive disk intact:\n%q", got)
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto a Pub/Sub profile: %q / %q",
			reloaded.Secret(model.SecretAccessKey), reloaded.Secret(model.SecretSecretKey))
	}

	// And the profile that would actually be dialled, which is the half a
	// stored-value check cannot reach: resolveACLCredentials runs there.
	dialled := recordDialledProfile(t, paths, settingsService, added.ID)
	if dialled.Auth.Mechanism != model.AuthPlain {
		t.Errorf("dialled with mechanism %q, want plain", dialled.Auth.Mechanism)
	}
	if dialled.Secret(model.SecretAccessKey) != "" || dialled.Secret(model.SecretSecretKey) != "" {
		t.Errorf("dialled with RocketMQ's global pair stamped on: %q / %q",
			dialled.Secret(model.SecretAccessKey), dialled.Secret(model.SecretSecretKey))
	}
	if dialled.Secret(pubsubdriver.SecretCredentialsJSON) != key {
		t.Errorf("dialled with %q, want the profile's own service account key",
			dialled.Secret(pubsubdriver.SecretCredentialsJSON))
	}
}
