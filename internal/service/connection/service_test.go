package connection

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/model"
)

func TestGetConnectionsReturnsCopies(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("original", "test", "ns:9876", 5, false, "", "", "")); err != nil {
		t.Fatal(err)
	}
	list := service.GetConnections()
	list[0].Name = "mutated"
	if got := service.GetConnections()[0].Name; got != "original" {
		t.Fatalf("stored connection was mutated through returned copy: %q", got)
	}
}

func TestResolveACLCredentialsConnectionWins(t *testing.T) {
	service := newTestService(t, fakeSettings{accessKey: "global-ak", secretKey: "global-sk"})
	enabled, accessKey, secretKey := service.resolveACLCredentials(&model.ConnectionProfile{
		Auth: model.AuthConfig{Mechanism: model.AuthACL},
		Secrets: map[string]string{
			model.SecretAccessKey: "connection-ak",
			model.SecretSecretKey: "connection-sk",
		},
	})
	if !enabled || accessKey != "connection-ak" || secretKey != "connection-sk" {
		t.Fatalf("got enabled=%v accessKey=%q secretKey=%q", enabled, accessKey, secretKey)
	}
}

func TestResolveACLCredentialsNoACLNoGlobal(t *testing.T) {
	service := newTestService(t, fakeSettings{})
	enabled, accessKey, secretKey := service.resolveACLCredentials(&model.ConnectionProfile{})
	if enabled || accessKey != "" || secretKey != "" {
		t.Fatalf("expected no ACL, got enabled=%v accessKey=%q secretKey=%q", enabled, accessKey, secretKey)
	}
}

func TestResolveACLCredentialsGlobalFallback(t *testing.T) {
	service := newTestService(t, fakeSettings{accessKey: "global-ak", secretKey: "global-sk"})
	enabled, accessKey, secretKey := service.resolveACLCredentials(&model.ConnectionProfile{})
	if !enabled || accessKey != "global-ak" || secretKey != "global-sk" {
		t.Fatalf("got enabled=%v accessKey=%q secretKey=%q", enabled, accessKey, secretKey)
	}
}

func TestResolveACLCredentialsRequiresCompleteGlobalPair(t *testing.T) {
	service := newTestService(t, fakeSettings{accessKey: "global-ak"})
	enabled, accessKey, secretKey := service.resolveACLCredentials(&model.ConnectionProfile{})
	if enabled || accessKey != "" || secretKey != "" {
		t.Fatalf("incomplete global credentials must be ignored, got enabled=%v accessKey=%q secretKey=%q", enabled, accessKey, secretKey)
	}
}

func TestConnectionCRUDAndDefault(t *testing.T) {
	service := newTestService(t, fakeSettings{connectTimeout: 3 * time.Second, autoConnect: true})
	first, err := service.AddConnection(profileOf("prod", "production", "ns1:9876", 5, false, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.Name != "prod" || !first.IsDefault {
		t.Fatalf("unexpected first connection: %#v", first)
	}
	second, err := service.AddConnection(profileOf("test", "test", "ns2:9876;ns3:9876", 8, true, "ak", "sk", "note"))
	if err != nil {
		t.Fatal(err)
	}
	if !second.ACLEnabled() || second.Secret(model.SecretAccessKey) != "ak" {
		t.Fatalf("unexpected ACL connection: %#v", second)
	}

	list := service.GetConnections()
	if len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("unexpected sorted connection list: %#v", list)
	}
	list[0].Name = "hacked"
	stored, err := service.GetConnection(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "prod" {
		t.Fatal("GetConnections returned mutable internal state")
	}

	updated, err := service.UpdateConnection(first.ID, profileOf("prod-2", "production", "ns1:9876", 6, false, "", "", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "prod-2" || updated.TimeoutSec != 6 {
		t.Fatalf("unexpected update: %#v", updated)
	}
	if err := service.SetDefaultConnection(second.ID); err != nil {
		t.Fatal(err)
	}
	defaultID := 0
	for _, connection := range service.GetConnections() {
		if connection.IsDefault {
			defaultID = connection.ID
		}
	}
	if defaultID != second.ID {
		t.Fatalf("default ID = %d, want %d", defaultID, second.ID)
	}
	if err := service.DeleteConnection(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetConnection(first.ID); err == nil {
		t.Fatal("deleted connection should not be found")
	}
}

func TestConnectionAddValidation(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("", "test", "ns:9876", 5, false, "", "", "")); err == nil {
		t.Fatal("empty name should fail")
	}
	if _, err := service.AddConnection(profileOf("x", "test", "", 5, false, "", "", "")); err == nil {
		t.Fatal("empty NameServer should fail")
	}
	if _, err := service.AddConnection(profileOf("x", "test", "ns:9876", 5, true, "", "sk", "")); err == nil {
		t.Fatal("ACL without AccessKey should fail")
	}
}

func TestConnectionPersistReload(t *testing.T) {
	service := newTestService(t, nil)
	if _, err := service.AddConnection(profileOf("keep", "development", "127.0.0.1:9876", 5, true, "ak1", "sk1", "")); err != nil {
		t.Fatal(err)
	}
	reloaded := New(service.dataFilePath, fakeSettings{connectTimeout: 3 * time.Second, autoConnect: true}, noopRuntime{}, addressedEndpoints{})
	list := reloaded.GetConnections()
	if len(list) != 1 || list[0].Secret(model.SecretAccessKey) != "ak1" || list[0].Secret(model.SecretSecretKey) != "sk1" {
		t.Fatalf("reloaded credentials do not match: %#v", list)
	}
}

/*
 * The bug: only RocketMQ's access key pair survived a save.
 *
 * Every other family's credentials were dropped on the way to disk, and the
 * auth mechanism was forced to "none" with them. A RabbitMQ connection was
 * therefore stored as anonymous with no username and no password, and could
 * not open. The form's test button hid it: it probes the submitted profile
 * rather than the stored one, so it passed on the way in.
 */
func TestAddKeepsEveryDriverSecret(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name:       "RabbitMQ",
		Kind:       model.KindRabbitMQ,
		Endpoints:  "http://127.0.0.1:15672",
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
	}
	input.SetSecret("username", "app")
	input.SetSecret("password", "s3cret")

	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("username") != "app" || stored.Secret("password") != "s3cret" {
		t.Errorf("stored username=%q password=%q, want both",
			stored.Secret("username"), stored.Secret("password"))
	}
	// A family with its own mechanism must not be filed as anonymous.
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism = %q, want %q", stored.Auth.Mechanism, model.AuthPlain)
	}
}

// And they have to survive the round trip through the file, which is where
// they are encrypted.
func TestDriverSecretsSurviveAReload(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "RabbitMQ", Kind: model.KindRabbitMQ,
		Endpoints: "http://127.0.0.1:15672", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}
	input.SetSecret("username", "app")
	input.SetSecret("password", "s3cret")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("password") != "s3cret" {
		t.Errorf("password after reload = %q", stored.Secret("password"))
	}
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q", stored.Auth.Mechanism)
	}
}

// Editing has the same hole, and a password nobody can change is as bad as one
// nobody can set.
func TestUpdateReplacesADriverSecret(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "RabbitMQ", Kind: model.KindRabbitMQ,
		Endpoints: "http://127.0.0.1:15672", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}
	input.SetSecret("username", "app")
	input.SetSecret("password", "old")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	input.SetSecret("password", "new")
	if _, err := service.UpdateConnection(added.ID, input); err != nil {
		t.Fatal(err)
	}
	stored, err := service.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("password") != "new" {
		t.Errorf("password = %q, want the new one", stored.Secret("password"))
	}
}

/*
 * RocketMQ's path is untouched. Turning ACL off still clears the pair and
 * files the connection as anonymous, which is what "no ACL" means for the one
 * family whose only mechanism is ACL.
 */
func TestDisablingACLStillClearsTheRocketMQPair(t *testing.T) {
	service := newTestService(t, nil)
	withACL := profileOf("rmq", "test", "ns:9876", 5, true, "ak", "sk", "")
	added, err := service.AddConnection(withACL)
	if err != nil {
		t.Fatal(err)
	}
	if added.Secret(model.SecretAccessKey) != "ak" || added.Auth.Mechanism != model.AuthACL {
		t.Fatalf("ACL was not stored: %+v", added.Auth)
	}

	without := profileOf("rmq", "test", "ns:9876", 5, false, "", "", "")
	updated, err := service.UpdateConnection(added.ID, without)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Secret(model.SecretAccessKey) != "" || updated.Secret(model.SecretSecretKey) != "" {
		t.Error("the access key pair survived ACL being turned off")
	}
	if updated.Auth.Mechanism != model.AuthNone {
		t.Errorf("mechanism = %q, want none", updated.Auth.Mechanism)
	}
}

/*
 * A changed credential has to drop the open client. It used to compare only
 * the access key pair, so a new RabbitMQ password left the old connection
 * running until the app restarted.
 */
func TestChangingADriverSecretForcesAReconnect(t *testing.T) {
	service := newTestService(t, nil)

	input := model.ConnectionProfile{
		Name: "RabbitMQ", Kind: model.KindRabbitMQ,
		Endpoints: "http://127.0.0.1:15672", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}
	input.SetSecret("username", "app")
	input.SetSecret("password", "old")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	input.SetSecret("password", "new")
	updated, err := service.UpdateConnection(added.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.StatusOffline {
		t.Errorf("status = %q, want offline so the next read redials", updated.Status)
	}
}

func TestDialParametersChangedCoversEveryDialInput(t *testing.T) {
	base := model.ConnectionProfile{
		Endpoints:  "127.0.0.1:9876",
		TimeoutSec: 5,
		Options:    map[string]string{"namespace": "ns"},
		Secrets:    map[string]string{"accessKey": "a"},
	}
	if dialParametersChanged(base, base) {
		t.Error("an unchanged profile reported a changed dial")
	}

	cases := map[string]func(*model.ConnectionProfile){
		"endpoints":  func(p *model.ConnectionProfile) { p.Endpoints = "127.0.0.1:9877" },
		"timeout":    func(p *model.ConnectionProfile) { p.TimeoutSec = 9 },
		"acl":        func(p *model.ConnectionProfile) { p.Auth.Mechanism = model.AuthACL },
		"option":     func(p *model.ConnectionProfile) { p.Options = map[string]string{"namespace": "other"} },
		"optionGone": func(p *model.ConnectionProfile) { p.Options = nil },
		"secret":     func(p *model.ConnectionProfile) { p.Secrets = map[string]string{"accessKey": "b"} },
	}
	for name, mutate := range cases {
		changed := base
		mutate(&changed)
		if !dialParametersChanged(base, changed) {
			t.Errorf("%s: a changed dial parameter reported no change", name)
		}
	}

	// Labels are not dial parameters: renaming must not drop a working client.
	renamed := base
	renamed.Name, renamed.Group, renamed.Remark = "other", "prod", "note"
	if dialParametersChanged(base, renamed) {
		t.Error("renaming reported a changed dial")
	}
}

/*
 * Kafka is the first family whose profile carries more than a credential in
 * its options, and the first where authenticating with nothing is a real
 * choice rather than a blank form. Both are new ways for the store to lose
 * something on the way to disk.
 *
 * The lesson from the RabbitMQ break above is that only the round trip tells
 * the truth: the form's test button probes the profile it was handed, so it
 * passes on the way in whatever the store then does with it.
 */
// A RocketMQ namespace decides which topics and groups a connection can even
// address, so losing it on the way to disk would leave a connection pointing
// at the whole cluster with no sign that anything changed. The form's test
// button probes the submitted profile rather than the stored one, so nothing
// in the app would have caught that.
func TestRocketMQNamespaceSurvivesAReload(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name:       "RocketMQ",
		Kind:       model.KindRocketMQ,
		Endpoints:  "ns-a:9876;ns-b:9876",
		TimeoutSec: 5,
		Options:    map[string]string{"namespace": "MQ_INST_1", "version": "5.x", "access": "ns"},
	}

	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Option("namespace"); got != "MQ_INST_1" {
		t.Errorf("namespace after reload = %q, want MQ_INST_1", got)
	}
	// An unscoped connection is the common case and must stay unscoped: an
	// empty option and a missing one both have to mean the whole cluster.
	delete(input.Options, "namespace")
	unscoped, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	reopened = New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err = reopened.GetConnection(unscoped.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Option("namespace"); got != "" {
		t.Errorf("namespace after reload = %q, want empty", got)
	}
}

func TestKafkaProfileSurvivesAReload(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name:       "Kafka",
		Kind:       model.KindKafka,
		Endpoints:  "kafka-1:9092,kafka-2:9092",
		TimeoutSec: 9,
		Auth:       model.AuthConfig{Mechanism: model.AuthSASLScram},
		Options: map[string]string{
			"scramSha":      "256",
			"tls":           "true",
			"tlsCaFile":     "/etc/kafka/ca.pem",
			"tlsSkipVerify": "true",
		},
	}
	input.SetSecret("username", "admin")
	input.SetSecret("password", "s3cret")

	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}

	if stored.Kind != model.KindKafka {
		t.Errorf("kind after reload = %q, want kafka", stored.Kind)
	}
	// The digest is not decoration: SHA-256 and SHA-512 are separate
	// credentials on the broker, so losing it authenticates as the wrong user.
	for key, want := range map[string]string{
		"scramSha":      "256",
		"tls":           "true",
		"tlsCaFile":     "/etc/kafka/ca.pem",
		"tlsSkipVerify": "true",
	} {
		if got := stored.Option(key); got != want {
			t.Errorf("option %q after reload = %q, want %q", key, got, want)
		}
	}
	if stored.Secret("username") != "admin" || stored.Secret("password") != "s3cret" {
		t.Errorf("stored username=%q password=%q, want both",
			stored.Secret("username"), stored.Secret("password"))
	}
	if stored.Auth.Mechanism != model.AuthSASLScram {
		t.Errorf("mechanism after reload = %q, want %q", stored.Auth.Mechanism, model.AuthSASLScram)
	}
	if stored.TimeoutSec != 9 {
		t.Errorf("timeout after reload = %d, want 9", stored.TimeoutSec)
	}
}

// An anonymous Kafka connection is a connection, not an unfinished form. It
// has to come back as one rather than being repaired into something else.
func TestAnonymousKafkaProfileSurvivesAReload(t *testing.T) {
	service := newTestService(t, nil)
	added, err := service.AddConnection(model.ConnectionProfile{
		Name:       "Kafka dev",
		Kind:       model.KindKafka,
		Endpoints:  "localhost:9092",
		TimeoutSec: 5,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
	})
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Auth.Mechanism != model.AuthNone {
		t.Errorf("mechanism after reload = %q, want none", stored.Auth.Mechanism)
	}
	if len(stored.ConfiguredSecrets()) != 0 {
		t.Errorf("anonymous connection carries secrets: %v", stored.ConfiguredSecrets())
	}
}

// Switching a stored SASL connection to anonymous has to take the credential
// with it. One left behind would be used again the moment SASL is re-selected,
// without the form ever having shown it.
func TestKafkaDroppingSASLClearsTheStoredCredential(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Kafka", Kind: model.KindKafka,
		Endpoints: "localhost:9092", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthSASLPlain},
	}
	input.SetSecret("username", "admin")
	input.SetSecret("password", "s3cret")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.UpdateConnection(added.ID, model.ConnectionProfile{
		Name: "Kafka", Kind: model.KindKafka,
		Endpoints: "localhost:9092", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthNone},
	}); err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Auth.Mechanism != model.AuthNone {
		t.Errorf("mechanism after reload = %q, want none", stored.Auth.Mechanism)
	}
	if stored.Secret("password") != "" {
		t.Errorf("the password outlived the mechanism that used it")
	}
}

/*
 * Pulsar's token has to survive the disk, not only the form.
 *
 * The connection form's test button probes the profile it was handed, never
 * the stored one, so a credential that is dropped on save passes on the way in
 * and fails on the next start. That is exactly how the pre-2026-08-31 layer
 * lost every family's secrets but RocketMQ's, and a driver whose whole
 * authentication is one bearer token has nothing left when it goes.
 */
func TestPulsarTokenSurvivesAReload(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Pulsar", Kind: model.KindPulsar,
		Endpoints: "pulsar://127.0.0.1:6650", TimeoutSec: 5,
		Auth:    model.AuthConfig{Mechanism: model.AuthToken},
		Options: map[string]string{"adminUrl": "http://127.0.0.1:8080"},
	}
	input.SetSecret("token", "a-jwt")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("token") != "a-jwt" {
		t.Errorf("token after reload = %q", stored.Secret("token"))
	}
	if stored.Auth.Mechanism != model.AuthToken {
		t.Errorf("mechanism after reload = %q, want token", stored.Auth.Mechanism)
	}
	// The two addresses are separate listeners, and the admin one lives in
	// Options. A profile that came back without it can dial and read nothing.
	if stored.Option("adminUrl") != "http://127.0.0.1:8080" {
		t.Errorf("admin URL after reload = %q", stored.Option("adminUrl"))
	}
}

// The token is stored encrypted, so the file must not be readable as one.
func TestPulsarTokenIsNotStoredInTheClear(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Pulsar", Kind: model.KindPulsar,
		Endpoints: "pulsar://127.0.0.1:6650", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthToken},
	}
	input.SetSecret("token", "a-very-secret-jwt")
	if _, err := service.AddConnection(input); err != nil {
		t.Fatal(err)
	}

	written, err := os.ReadFile(service.dataFilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "a-very-secret-jwt") {
		t.Error("the token is on disk in the clear")
	}
}

// The address field is named for every family, so the message when it is empty
// has to be too. A Kafka user told to fill in a NameServer has been sent to
// look for a field that is not on the form.
func TestEmptyEndpointsAreNotReportedAsAMissingNameServer(t *testing.T) {
	service := newTestService(t, nil)
	_, err := service.AddConnection(model.ConnectionProfile{
		Name: "Kafka", Kind: model.KindKafka, Endpoints: "  ", TimeoutSec: 5,
	})
	if err == nil {
		t.Fatal("a profile with no address was accepted")
	}
	if strings.Contains(err.Error(), "NameServer") {
		t.Errorf("error names a RocketMQ field to a kafka connection: %v", err)
	}
}

/*
 * Clearing a credential has to actually remove it.
 *
 * applyCredentials only ever wrote the secrets it was handed, so a stored one
 * the submission no longer carries stayed on the profile. SetACL hid it for
 * RocketMQ by deleting the access key pair by name; every other family's
 * credential simply survived being cleared, and the next connect used a
 * password the form had reported as gone.
 */
func TestClearingCredentialsRemovesThem(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "RabbitMQ", Kind: model.KindRabbitMQ,
		Endpoints: "http://127.0.0.1:15672", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}
	input.SetSecret("username", "app")
	input.SetSecret("password", "s3cret")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	// What the bridge submits for credentialsMode "clear": the same profile
	// with no secrets on it at all.
	if _, err := service.UpdateConnection(added.ID, model.ConnectionProfile{
		Name: "RabbitMQ", Kind: model.KindRabbitMQ,
		Endpoints: "http://127.0.0.1:15672", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthNone},
	}); err != nil {
		t.Fatal(err)
	}

	stored, err := service.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.ConfiguredSecrets()) != 0 {
		t.Errorf("cleared credentials are still stored: %v", stored.ConfiguredSecrets())
	}
}

// The other half of the same rule: a submission that carries a credential
// replaces the stored one rather than being merged into it, so removing one
// key of a pair removes it.
func TestReplacingCredentialsDropsTheKeysNoLongerSent(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Kafka", Kind: model.KindKafka,
		Endpoints: "localhost:9092", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthSASLPlain},
	}
	input.SetSecret("username", "admin")
	input.SetSecret("password", "s3cret")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	replacement := model.ConnectionProfile{
		Name: "Kafka", Kind: model.KindKafka,
		Endpoints: "localhost:9092", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthSASLPlain},
	}
	replacement.SetSecret("username", "admin")
	if _, err := service.UpdateConnection(added.ID, replacement); err != nil {
		t.Fatal(err)
	}

	stored, err := service.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("username") != "admin" {
		t.Errorf("username = %q, want admin", stored.Secret("username"))
	}
	if stored.Secret("password") != "" {
		t.Errorf("password was not sent but is still stored")
	}
}

/*
 * A Redis profile has to survive the disk round trip whole.
 *
 * This is the check the RabbitMQ integration learned to make the hard way:
 * the connection layer once stored only RocketMQ's access key pair, so every
 * other family's credential was dropped on save and its auth mechanism forced
 * back to none. Nothing pointed at it, because the connection form's test
 * button probes the profile that was submitted rather than the one that was
 * stored - it passed on the way in and the connection failed afterwards.
 *
 * Redis brings a second thing worth pinning beside the credential: the options
 * are what decide which client go-redis builds. A deployment or a master name
 * lost in storage does not fail to connect - it connects to the wrong thing.
 */
func TestRedisStreamProfileSurvivesAReload(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Redis Stream", Kind: model.KindRedisStream,
		Endpoints: "s1:26379,s2:26379", TimeoutSec: 9,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			"deployment":    "sentinel",
			"masterName":    "mymaster",
			"db":            "4",
			"streamFilter":  "orders:*",
			"tls":           "true",
			"tlsSkipVerify": "true",
		},
	}
	input.SetSecret("username", "mqstudio")
	input.SetSecret("password", "s3cret")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}

	if stored.Secret("username") != "mqstudio" {
		t.Errorf("username after reload = %q", stored.Secret("username"))
	}
	if stored.Secret("password") != "s3cret" {
		t.Errorf("password after reload = %q", stored.Secret("password"))
	}
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain", stored.Auth.Mechanism)
	}
	for key, want := range input.Options {
		if got := stored.Option(key); got != want {
			t.Errorf("option %q after reload = %q, want %q", key, got, want)
		}
	}
	if stored.Endpoints != input.Endpoints {
		t.Errorf("endpoints after reload = %q", stored.Endpoints)
	}
	if stored.TimeoutSec != 9 {
		t.Errorf("timeout after reload = %d", stored.TimeoutSec)
	}
}

// Redis before 6.0 has no users, so a password with no username is a whole
// credential. A connection layer that treated a pair as all-or-nothing would
// store neither, and the profile would connect anonymously to a server that
// requires a password.
func TestRedisStreamKeepsAPasswordWithNoUsername(t *testing.T) {
	service := newTestService(t, nil)
	input := model.ConnectionProfile{
		Name: "Redis 5", Kind: model.KindRedisStream,
		Endpoints: "127.0.0.1:6379",
		Auth:      model.AuthConfig{Mechanism: model.AuthPlain},
		Options:   map[string]string{"deployment": "standalone"},
	}
	input.SetSecret("password", "only-a-password")
	added, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}

	reopened := New(service.dataFilePath, fakeSettings{}, noopRuntime{}, addressedEndpoints{})
	stored, err := reopened.GetConnection(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret("password") != "only-a-password" {
		t.Errorf("password after reload = %q", stored.Secret("password"))
	}
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism after reload = %q, want plain", stored.Auth.Mechanism)
	}
}

/*
 * A profile is dialled with the mechanism it was stored with.
 *
 * SetACL owns the mechanism, which is right for a family whose only one is
 * ACL: off means anonymous. applyCredentials already restores a family's own
 * mechanism on the way in, and this path did not - so a profile was stored
 * correctly and then handed to the driver with its mechanism reset to none.
 *
 * It went unnoticed because every family that had a mechanism until now reads
 * its credentials straight out of the secret map and ignores the mechanism.
 * NATS is the first that reads it, because six of its mechanisms are
 * different kinds of credential rather than different names for a password -
 * an nkey seed is signed, a creds file is a path, a token is neither.
 */
func TestDiallingKeepsTheStoredMechanism(t *testing.T) {
	service := newTestService(t, nil)
	profile := &model.ConnectionProfile{
		Name: "NATS", Kind: model.KindNATS,
		Endpoints: "nats://127.0.0.1:4222", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}
	profile.SetSecret("username", "app")
	profile.SetSecret("password", "s3cret")

	resolved := service.resolveForDial(profile)
	if resolved.Auth.Mechanism != model.AuthPlain {
		t.Errorf("dialled with %q, want plain - the driver would authenticate as nobody",
			resolved.Auth.Mechanism)
	}
	if resolved.Secret("password") != "s3cret" {
		t.Error("the password did not survive being resolved for dialling")
	}
}

/*
 * The global access key pair is RocketMQ's, and it fills in a profile that
 * named no mechanism of its own.
 *
 * A profile that named one keeps it. Stamping ACL over a NATS connection
 * because the settings page happens to hold RocketMQ credentials would dial it
 * with a mechanism its driver does not implement, using a different broker's
 * credentials - and it would do so only for the users who configured them,
 * which is the worst way for it to fail.
 */
func TestGlobalACLCredentialsLeaveAnotherFamilyAlone(t *testing.T) {
	service := newTestService(t, fakeSettings{accessKey: "global-ak", secretKey: "global-sk"})
	profile := &model.ConnectionProfile{
		Name: "NATS", Kind: model.KindNATS,
		Endpoints: "nats://127.0.0.1:4222", TimeoutSec: 5,
		Auth: model.AuthConfig{Mechanism: model.AuthPlain},
	}

	resolved := service.resolveForDial(profile)
	if resolved.Auth.Mechanism != model.AuthPlain {
		t.Errorf("dialled with %q, want plain", resolved.Auth.Mechanism)
	}
	if resolved.Secret(model.SecretAccessKey) != "" {
		t.Error("RocketMQ's global access key was stamped onto a NATS connection")
	}

	// And the fallback still applies where it belongs: a profile that named
	// no mechanism is what the setting exists for.
	rocket := &model.ConnectionProfile{Name: "RocketMQ", Kind: model.KindRocketMQ}
	filled := service.resolveForDial(rocket)
	if filled.Auth.Mechanism != model.AuthACL || filled.Secret(model.SecretAccessKey) != "global-ak" {
		t.Errorf("the global pair no longer fills in a profile with no mechanism: %q / %q",
			filled.Auth.Mechanism, filled.Secret(model.SecretAccessKey))
	}
}

// Editing an option through the form has to redial too. It used to not:
// clientConfigChanged watched the endpoints, the timeout and the credentials
// only, so a namespace changed on an online connection was saved and then
// ignored until the app restarted.
func TestUpdateConnectionRedialsWhenOnlyAnOptionChanged(t *testing.T) {
	service := newTestService(t, fakeSettings{connectTimeout: 3 * time.Second, autoConnect: true})
	var resolved model.ConnectionProfile
	runtime := newRecordingRuntime()
	service.runtime = &capturingRuntime{recordingRuntime: runtime, seen: &resolved}

	input := profileOf("p", "", "ns:9876", 5, false, "", "", "")
	input.SetOption("namespace", "before")
	profile, err := service.AddConnection(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Connect(profile.ID); err != nil {
		t.Fatal(err)
	}

	edited := profileOf("p", "", "ns:9876", 5, false, "", "", "")
	edited.SetOption("namespace", "after")
	if _, err := service.UpdateConnection(profile.ID, edited); err != nil {
		t.Fatal(err)
	}
	if resolved.Option("namespace") != "after" {
		t.Fatalf("runtime dialled with namespace %q, want the edited one", resolved.Option("namespace"))
	}
}
