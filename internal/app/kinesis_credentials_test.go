package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
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
 * It is written again for Kinesis rather than borrowed from SQS, and the
 * reason is that nothing keeps the two in step: each driver declares its own
 * three constants, and the strings being equal today is a coincidence a rename
 * in either package would end. The test that would catch that rename is this
 * one, in this package, naming this driver's constants.
 *
 * The three secrets are the shape worth exercising: a session token is only
 * ever set on temporary credentials, so a store that dropped an empty one
 * would look correct in every test that filled all three.
 */
func TestKinesisCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindKinesis); !ok {
		driver.Register(kinesisdriver.New())
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

	const accessKeyID = "AKIAIOSFODNN7EXAMPLE"
	const secretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	const sessionToken = "FwoGZXIvYXdzEBYaDEXAMPLEsessionTOKEN//////////wEXAMPLE="

	input := model.ConnectionProfile{
		Name:       "kinesis with a global acl pair configured",
		Kind:       model.KindKinesis,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			kinesisdriver.OptionRegion:       "eu-west-1",
			kinesisdriver.OptionStreamPrefix: "team-",
		},
	}
	input.SetSecret(kinesisdriver.SecretAccessKeyID, accessKeyID)
	input.SetSecret(kinesisdriver.SecretSecretAccessKey, secretKey)
	input.SetSecret(kinesisdriver.SecretSessionToken, sessionToken)

	added, err := connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	// The half every dialled family has and this one must not: a profile that
	// saved with an address would mean the descriptor grew an endpoint field.
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

	if got := reloaded.Secret(kinesisdriver.SecretAccessKeyID); got != accessKeyID {
		t.Errorf("the access key id did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(kinesisdriver.SecretSecretAccessKey); got != secretKey {
		t.Errorf("the secret access key did not survive disk intact: %q", got)
	}
	// The third one, which the other two would pass without: a session token
	// carries slashes and an equals sign, and is the credential a store that
	// trimmed or re-encoded its values would break first.
	if got := reloaded.Secret(kinesisdriver.SecretSessionToken); got != sessionToken {
		t.Errorf("the session token did not survive disk intact: %q", got)
	}
	if reloaded.Option(kinesisdriver.OptionRegion) != "eu-west-1" {
		t.Errorf("the region did not survive disk: %q", reloaded.Option(kinesisdriver.OptionRegion))
	}
	if reloaded.Option(kinesisdriver.OptionStreamPrefix) != "team-" {
		t.Errorf("the stream prefix did not survive disk: %q",
			reloaded.Option(kinesisdriver.OptionStreamPrefix))
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto a Kinesis profile: %q / %q",
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
	if dialled.Secret(kinesisdriver.SecretAccessKeyID) != accessKeyID {
		t.Errorf("dialled with %q, want the profile's own access key id",
			dialled.Secret(kinesisdriver.SecretAccessKeyID))
	}
	if dialled.Secret(kinesisdriver.SecretSessionToken) != sessionToken {
		t.Error("dialled without the profile's own session token")
	}
	if dialled.Endpoints != "" {
		t.Errorf("dialled against %q, and this family has no address", dialled.Endpoints)
	}
}

/*
 * And the endpoint policy from the other side.
 *
 * The connection service reads DriverDescriptor.RequiresEndpoints to decide
 * whether a profile may save with an empty address, and this family declares
 * no endpoint field at all - so a profile with nothing but a region and a
 * credential has to save. It is the opposite of the same assertion for Service
 * Bus, and worth making because the rule is read off the driver rather than
 * written down anywhere central.
 */
func TestKinesisProfileSavesWithNoAddress(t *testing.T) {
	if _, ok := driver.Lookup(model.KindKinesis); !ok {
		driver.Register(kinesisdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())

	profile := model.ConnectionProfile{
		Name:       "kinesis with a region and nothing else",
		Kind:       model.KindKinesis,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options:    map[string]string{kinesisdriver.OptionRegion: "eu-west-1"},
	}

	added, err := connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("refused a Kinesis profile with no address, and the family has none: %v", err)
	}
	if added.Endpoints != "" {
		t.Errorf("the connection service invented an address: %q", added.Endpoints)
	}
	// No credential either, which is the machine's own AWS identity rather
	// than an unfinished form - so it must save the same way.
	if added.Auth.Mechanism != model.AuthNone {
		t.Errorf("mechanism = %q, want none for a profile that will use the default chain",
			added.Auth.Mechanism)
	}
}
