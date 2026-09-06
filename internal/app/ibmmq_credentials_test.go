package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	ibmmqdriver "github.com/amigoer/mq-studio/internal/driver/ibmmq"
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
 * It is written again for IBM MQ rather than borrowed from ActiveMQ, and the
 * reason is that nothing keeps the two in step: each driver declares its own
 * constants, and the strings being equal today is a coincidence a rename in
 * either package would end.
 *
 * Four secrets rather than two, and the second pair is what makes this worth
 * doing here. mqweb authorises its administrative and messaging interfaces
 * against two roles, so a deployment may hold them on two accounts - and a
 * store that dropped the second pair would produce a connection that reads
 * every board and cannot browse one message, which is exactly what a working
 * connection with the wrong role looks like.
 */
func TestIBMMQCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindIBMMQ); !ok {
		driver.Register(ibmmqdriver.New())
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

	const address = "https://mq.example.com:9443"
	const adminUser = "mqadmin"
	// Deliberately awkward: an mqweb password is whatever the registry holds,
	// and a store that trimmed or re-encoded its values would break on these
	// characters first.
	const adminPassword = "p@ss w0rd/=+"
	const messagingUser = "mqapp"
	const messagingPassword = "an0ther/=+ secret"

	input := model.ConnectionProfile{
		Name:       "ibm mq with a global acl pair configured",
		Kind:       model.KindIBMMQ,
		Endpoints:  address,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			ibmmqdriver.OptionQueueManager:  "QM1",
			ibmmqdriver.OptionTLSSkipVerify: "true",
		},
	}
	input.SetSecret(ibmmqdriver.SecretUsername, adminUser)
	input.SetSecret(ibmmqdriver.SecretPassword, adminPassword)
	input.SetSecret(ibmmqdriver.SecretMessagingUsername, messagingUser)
	input.SetSecret(ibmmqdriver.SecretMessagingPassword, messagingPassword)

	added, err := connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	// The half the hosted families do not have: this one dials a real address
	// and the profile has to keep it.
	if added.Endpoints != address {
		t.Fatalf("the saved profile carries %q, want the mqweb address", added.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())
	reloaded, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}

	if got := reloaded.Secret(ibmmqdriver.SecretUsername); got != adminUser {
		t.Errorf("the administrative user did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(ibmmqdriver.SecretPassword); got != adminPassword {
		t.Errorf("the administrative password did not survive disk intact: %q", got)
	}
	// The pair that would look fine in every test that only filled the first
	// one, and whose loss produces a connection that reads every board and
	// cannot browse a message.
	if got := reloaded.Secret(ibmmqdriver.SecretMessagingUsername); got != messagingUser {
		t.Errorf("the messaging user did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(ibmmqdriver.SecretMessagingPassword); got != messagingPassword {
		t.Errorf("the messaging password did not survive disk intact: %q", got)
	}
	if reloaded.Option(ibmmqdriver.OptionQueueManager) != "QM1" {
		t.Errorf("the queue manager did not survive disk: %q",
			reloaded.Option(ibmmqdriver.OptionQueueManager))
	}
	if reloaded.Option(ibmmqdriver.OptionTLSSkipVerify) != "true" {
		t.Errorf("the skip-verify switch did not survive disk: %q",
			reloaded.Option(ibmmqdriver.OptionTLSSkipVerify))
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto an IBM MQ profile: %q / %q",
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
	if dialled.Secret(ibmmqdriver.SecretUsername) != adminUser {
		t.Errorf("dialled with %q, want the profile's own administrative user",
			dialled.Secret(ibmmqdriver.SecretUsername))
	}
	if dialled.Secret(ibmmqdriver.SecretMessagingPassword) != messagingPassword {
		t.Error("dialled without the profile's own messaging password")
	}
	if dialled.Endpoints != address {
		t.Errorf("dialled against %q, want the mqweb address", dialled.Endpoints)
	}
}

/*
 * And the endpoint policy from the other side.
 *
 * The connection service reads DriverDescriptor.RequiresEndpoints to decide
 * whether a profile may save with an empty address, and this family declares
 * a required endpoint field - so a profile with nothing but a queue manager
 * and a credential must be refused. It is the opposite of the same assertion
 * for Kinesis, and worth making because the rule is read off the driver rather
 * than written down anywhere central: it would be easy to file IBM MQ with the
 * hosted families on the strength of its queue manager and channel names.
 */
func TestIBMMQProfileWillNotSaveWithoutAnAddress(t *testing.T) {
	if _, ok := driver.Lookup(model.KindIBMMQ); !ok {
		driver.Register(ibmmqdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())

	profile := model.ConnectionProfile{
		Name:       "ibm mq with a queue manager and no server",
		Kind:       model.KindIBMMQ,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options:    map[string]string{ibmmqdriver.OptionQueueManager: "QM1"},
	}

	if _, err := connections.AddConnection(profile); err == nil {
		t.Fatal("saved an IBM MQ profile with no mqweb address, and there is nothing to dial")
	}

	// With the address it saves, and the queue manager stays optional: most
	// installations front one and the driver discovers it.
	profile.Endpoints = "https://mq.example.com:9443"
	profile.Options = nil
	added, err := connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("refused a profile that names the server and lets the driver find "+
			"the queue manager: %v", err)
	}
	if added.Option(ibmmqdriver.OptionQueueManager) != "" {
		t.Errorf("the connection service invented a queue manager: %q",
			added.Option(ibmmqdriver.OptionQueueManager))
	}
}
