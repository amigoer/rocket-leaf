package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	solacedriver "github.com/amigoer/mq-studio/internal/driver/solace"
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
 * It is written again for Solace rather than borrowed from IBM MQ, and the
 * reason is that nothing keeps the two in step: each driver declares its own
 * constants, and the strings being equal today is a coincidence a rename in
 * either package would end.
 *
 * Four secrets rather than two, and the second pair means something different
 * here than IBM MQ's does. Both of that family's interfaces authenticate
 * against one mqweb registry, so an empty second pair falls back to the first.
 * Solace's two are a broker-wide management user and a client username inside
 * one Message VPN, so there is no fallback at all - which makes losing the
 * second pair on the way to disk a connection that reads every board and
 * cannot send, with nothing to explain it.
 */
func TestSolaceCredentialsSurviveGlobalACLSettings(t *testing.T) {
	if _, ok := driver.Lookup(model.KindSolace); !ok {
		driver.Register(solacedriver.New())
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

	const address = "http://solace.example.com:8080"
	const restAddress = "http://solace.example.com:9000"
	const managementUser = "sempadmin"
	// Deliberately awkward: a SEMP password is whatever the broker's user
	// store holds, and a profile store that trimmed or re-encoded its values
	// would break on these characters first.
	const managementPassword = "p@ss w0rd/=+"
	const clientUser = "orders-service"
	const clientPassword = "an0ther/=+ secret"
	// A Message VPN name with a slash in it, which is ordinary here and is
	// also the shape that breaks anything joining the scope onto a path.
	const vpn = "orders/eu"

	input := model.ConnectionProfile{
		Name:       "solace with a global acl pair configured",
		Kind:       model.KindSolace,
		Endpoints:  address,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			solacedriver.OptionMsgVPN:        vpn,
			solacedriver.OptionRESTURL:       restAddress,
			solacedriver.OptionTLSSkipVerify: "true",
		},
	}
	input.SetSecret(solacedriver.SecretUsername, managementUser)
	input.SetSecret(solacedriver.SecretPassword, managementPassword)
	input.SetSecret(solacedriver.SecretRESTUsername, clientUser)
	input.SetSecret(solacedriver.SecretRESTPassword, clientPassword)

	added, err := connections.AddConnection(input)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	// The half the hosted families do not have: this one dials a real address
	// and the profile has to keep it.
	if added.Endpoints != address {
		t.Fatalf("the saved profile carries %q, want the semp address", added.Endpoints)
	}

	// A second service on the same file, which is what a restart is.
	reopened := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())
	reloaded, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatalf("read the stored profile: %v", err)
	}

	if got := reloaded.Secret(solacedriver.SecretUsername); got != managementUser {
		t.Errorf("the management user did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(solacedriver.SecretPassword); got != managementPassword {
		t.Errorf("the management password did not survive disk intact: %q", got)
	}
	// The pair that would look fine in every test that only filled the first
	// one, and whose loss produces a connection that reads every board and
	// cannot send a message.
	if got := reloaded.Secret(solacedriver.SecretRESTUsername); got != clientUser {
		t.Errorf("the client username did not survive disk intact: %q", got)
	}
	if got := reloaded.Secret(solacedriver.SecretRESTPassword); got != clientPassword {
		t.Errorf("the client password did not survive disk intact: %q", got)
	}
	// The scope, which every path the driver builds is made from. A profile
	// that lost it reads an entirely different set of objects and reports
	// nothing wrong.
	if reloaded.Option(solacedriver.OptionMsgVPN) != vpn {
		t.Errorf("the message vpn did not survive disk: %q",
			reloaded.Option(solacedriver.OptionMsgVPN))
	}
	if reloaded.Option(solacedriver.OptionRESTURL) != restAddress {
		t.Errorf("the rest messaging address did not survive disk: %q",
			reloaded.Option(solacedriver.OptionRESTURL))
	}
	if reloaded.Option(solacedriver.OptionTLSSkipVerify) != "true" {
		t.Errorf("the skip-verify switch did not survive disk: %q",
			reloaded.Option(solacedriver.OptionTLSSkipVerify))
	}
	if reloaded.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain; the global pair rewrote it",
			reloaded.Auth.Mechanism)
	}
	if reloaded.Secret(model.SecretAccessKey) != "" || reloaded.Secret(model.SecretSecretKey) != "" {
		t.Errorf("the reserved RocketMQ ACL names were written onto a Solace profile: %q / %q",
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
	if dialled.Secret(solacedriver.SecretUsername) != managementUser {
		t.Errorf("dialled with %q, want the profile's own management user",
			dialled.Secret(solacedriver.SecretUsername))
	}
	if dialled.Secret(solacedriver.SecretRESTPassword) != clientPassword {
		t.Error("dialled without the profile's own client password")
	}
	if dialled.Option(solacedriver.OptionMsgVPN) != vpn {
		t.Errorf("dialled against message vpn %q, want %q",
			dialled.Option(solacedriver.OptionMsgVPN), vpn)
	}
	if dialled.Endpoints != address {
		t.Errorf("dialled against %q, want the semp address", dialled.Endpoints)
	}
}

/*
 * And the endpoint policy from the other side.
 *
 * The connection service reads DriverDescriptor.RequiresEndpoints to decide
 * whether a profile may save with an empty address, and this family declares a
 * required endpoint field - so a profile with nothing but a Message VPN and a
 * credential must be refused. It is the opposite of the same assertion for
 * Kinesis, and worth making because the rule is read off the driver rather
 * than written down anywhere central: it would be easy to file Solace with the
 * hosted families on the strength of Solace Cloud.
 */
func TestSolaceProfileWillNotSaveWithoutAnAddress(t *testing.T) {
	if _, ok := driver.Lookup(model.KindSolace); !ok {
		driver.Register(solacedriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, noDialRuntime{}, newDescriptorEndpoints())

	profile := model.ConnectionProfile{
		Name:       "solace with a message vpn and no broker",
		Kind:       model.KindSolace,
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options:    map[string]string{solacedriver.OptionMsgVPN: "orders"},
	}

	if _, err := connections.AddConnection(profile); err == nil {
		t.Fatal("saved a Solace profile with no semp address, and there is nothing to dial")
	}

	// With the address it saves, and the Message VPN stays optional: every
	// broker ships one called "default" and the driver falls back to it.
	profile.Endpoints = "http://solace.example.com:8080"
	profile.Options = nil
	added, err := connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("refused a profile that names the broker and lets the driver settle "+
			"the message vpn: %v", err)
	}
	if added.Option(solacedriver.OptionMsgVPN) != "" {
		t.Errorf("the connection service invented a message vpn: %q",
			added.Option(solacedriver.OptionMsgVPN))
	}
}
