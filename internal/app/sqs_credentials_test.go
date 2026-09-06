package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
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
 * reusing those names would have its credentials cleared on save and
 * RocketMQ's global pair stamped on at dial time - and nothing else would go
 * red, because the profile would still save, still reload and still look
 * complete.
 *
 * So the settings here hold a global pair, which is the state in which it
 * would bite, and the assertion is the whole round trip: an SQS profile with
 * no address saves, comes back off a second service reading the same file with
 * its own credentials intact, keeps its own mechanism, and is resolved for
 * dialling without the reserved names being filled in behind it.
 *
 * It lives here rather than in internal/service/connection because this is
 * where the two halves meet: the composition root binds the endpoint policy to
 * the driver's own descriptor, and it is that descriptor - with no endpoint
 * field on it - that makes an empty address savable at all.
 */
func TestSQSCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindSQS); !ok {
		driver.Register(sqsdriver.New())
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

	input := model.ConnectionProfile{
		Name:       "sqs with a global acl pair configured",
		Kind:       model.KindSQS,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{sqsdriver.OptionRegion: "eu-west-1"},
	}
	input.SetSecret(sqsdriver.SecretAccessKeyID, "AKIA-example")
	input.SetSecret(sqsdriver.SecretSecretAccessKey, "s3cret")
	input.SetSecret(sqsdriver.SecretSessionToken, "session-token")

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

	for _, want := range []struct{ key, value string }{
		{sqsdriver.SecretAccessKeyID, "AKIA-example"},
		{sqsdriver.SecretSecretAccessKey, "s3cret"},
		{sqsdriver.SecretSessionToken, "session-token"},
	} {
		if got := reloaded.Secret(want.key); got != want.value {
			t.Errorf("%s after reload = %q, want %q", want.key, got, want.value)
		}
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto an SQS profile: %q / %q",
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
	if dialled.Secret(sqsdriver.SecretAccessKeyID) != "AKIA-example" {
		t.Errorf("dialled with %q, want the profile's own access key id",
			dialled.Secret(sqsdriver.SecretAccessKeyID))
	}
}

// noDialRuntime stands in where the test only exercises profile persistence.
type noDialRuntime struct{}

func (noDialRuntime) Connect(model.ConnectionProfile) error { return nil }
func (noDialRuntime) HasClient(int) bool                    { return false }
func (noDialRuntime) Remove(int)                            {}
func (noDialRuntime) Test(model.ConnectionProfile) error    { return nil }
func (noDialRuntime) CloseAll()                             {}

// recordingRuntime captures the profile the connection service hands the
// driver, which is the only place the reserved-name resolution can be seen.
type recordingRuntime struct {
	noDialRuntime
	dialled model.ConnectionProfile
}

func (r *recordingRuntime) Connect(profile model.ConnectionProfile) error {
	r.dialled = profile
	return nil
}

func recordDialledProfile(
	t *testing.T, paths layout.Layout, settingsService *settings.Service, connID int,
) model.ConnectionProfile {
	t.Helper()
	runtime := &recordingRuntime{}
	service := connection.New(
		paths.ConnectionsFile, settingsService, runtime, newDescriptorEndpoints())
	if err := service.Connect(connID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return runtime.dialled
}
