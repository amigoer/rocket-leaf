package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	servicebusdriver "github.com/amigoer/mq-studio/internal/driver/azureservicebus"
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
 * would bite, and the assertion is the whole round trip: a Service Bus profile
 * saves, comes back off a second service reading the same file with both its
 * own secrets intact, keeps its own mechanism, and is resolved for dialling
 * without the reserved names being filled in behind it.
 *
 * Two secrets rather than one, which is what makes this family's version of
 * the test worth writing again. A shared access key and a whole connection
 * string are different lengths, different shapes and stored under two names,
 * and a store that kept one and dropped the other would produce a profile that
 * saves, reloads, looks right and cannot sign.
 *
 * The endpoint is the other half. This is the first hosted family with one, so
 * unlike SQS and Pub/Sub the round trip has to keep an address as well - and
 * the connection service's endpoint policy comes from the driver's own
 * descriptor, which is what makes this the place the two halves meet.
 */
func TestAzureServiceBusCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindAzureServiceBus); !ok {
		driver.Register(servicebusdriver.New())
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

	// A shared access key as the portal prints one: base64, 44 characters,
	// with the trailing padding that a store trimming its values would eat.
	const key = "aBcD1234eFgH5678iJkL9012mNoP3456qRsT7890uVw="
	const connectionString = "Endpoint=sb://orders.servicebus.windows.net/;" +
		"SharedAccessKeyName=Reader;SharedAccessKey=" + key

	input := model.ConnectionProfile{
		Name:       "service bus with a global acl pair configured",
		Kind:       model.KindAzureServiceBus,
		Endpoints:  "orders.servicebus.windows.net",
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			servicebusdriver.OptionSharedAccessKeyName: "Reader",
			servicebusdriver.OptionEntityPrefix:        "team-",
		},
	}
	input.SetSecret(servicebusdriver.SecretSharedAccessKey, key)
	input.SetSecret(servicebusdriver.SecretConnectionString, connectionString)

	added, err := connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	// The half SQS and Pub/Sub have no version of: this family is reached by
	// dialling something, so the address has to survive too.
	if added.Endpoints != "orders.servicebus.windows.net" {
		t.Fatalf("the saved profile's namespace is %q", added.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())
	reloaded, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}

	if got := reloaded.Secret(servicebusdriver.SecretSharedAccessKey); got != key {
		t.Errorf("the shared access key did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(servicebusdriver.SecretConnectionString); got != connectionString {
		t.Errorf("the connection string did not survive disk intact: %q", got)
	}
	if reloaded.Endpoints != "orders.servicebus.windows.net" {
		t.Errorf("the namespace did not survive disk: %q", reloaded.Endpoints)
	}
	if reloaded.Option(servicebusdriver.OptionSharedAccessKeyName) != "Reader" {
		t.Errorf("the policy name did not survive disk: %q",
			reloaded.Option(servicebusdriver.OptionSharedAccessKeyName))
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto a Service Bus profile: %q / %q",
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
	if dialled.Secret(servicebusdriver.SecretSharedAccessKey) != key {
		t.Errorf("dialled with %q, want the profile's own shared access key",
			dialled.Secret(servicebusdriver.SecretSharedAccessKey))
	}
	if dialled.Secret(servicebusdriver.SecretConnectionString) != connectionString {
		t.Errorf("dialled without the profile's own connection string")
	}
	if dialled.Endpoints != "orders.servicebus.windows.net" {
		t.Errorf("dialled against %q", dialled.Endpoints)
	}
}

/*
 * And the endpoint policy from the other side.
 *
 * The connection service reads DriverDescriptor.RequiresEndpoints to decide
 * whether a profile may save with an empty address. This family declares an
 * endpoint field and requires it, so a profile without one has to be refused -
 * which is the opposite of what the same test asserts for SQS and Pub/Sub, and
 * the reason it is worth asserting at all: the rule is read off the driver
 * rather than written down anywhere central.
 */
func TestAzureServiceBusProfileNeedsItsNamespace(t *testing.T) {
	if _, ok := driver.Lookup(model.KindAzureServiceBus); !ok {
		driver.Register(servicebusdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())

	profile := model.ConnectionProfile{
		Name:       "service bus with no namespace",
		Kind:       model.KindAzureServiceBus,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
	}
	profile.SetSecret(servicebusdriver.SecretSharedAccessKey, "aBcD=")

	if _, err := connections.AddConnection(profile); err == nil {
		t.Fatal("saved a Service Bus profile with no namespace, and both its clients dial one")
	}
}
