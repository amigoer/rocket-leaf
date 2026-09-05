package app

import (
	"testing"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/driver/pulsar"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

const (
	livePulsarService = "pulsar://127.0.0.1:6650"
	livePulsarAdmin   = "http://127.0.0.1:8080"
)

func requireLivePulsar(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Name:   "pulsar",
		Family: e2e.Pulsar,
		Start:  "npm run e2e:pulsar:up",
		Probe:  e2e.HTTPGet(livePulsarAdmin + "/admin/v2/brokers/health"),
	})
}

// livePulsarStack assembles the same pieces New does, rooted in a temp
// directory so a test never touches the developer's own connections file.
func livePulsarStack(t *testing.T) (*connection.Service, *driver.Registry) {
	t.Helper()
	requireLivePulsar(t)
	if _, ok := driver.Lookup(model.KindPulsar); !ok {
		driver.Register(pulsar.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	settingsService := settings.New(paths.SettingsFile)
	connections := connection.New(
		paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints())
	return connections, registry
}

// livePulsarProfile is what the connection form submits for the compose
// cluster: two addresses and a scope, which is the whole Pulsar form.
func livePulsarProfile(name string) model.ConnectionProfile {
	return model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindPulsar,
		Endpoints:  livePulsarService,
		TimeoutSec: 10,
		Options: map[string]string{
			pulsar.OptionAdminURL:  livePulsarAdmin,
			pulsar.OptionTenant:    "public",
			pulsar.OptionNamespace: "default",
		},
	}
}

/*
 * The whole path the connection dialog drives, against a real cluster.
 *
 * The driver's own live tests call Open directly. This is the layer between
 * that and the button: the profile goes through validation, encryption and the
 * registry, and comes back as a stored connection the boards read by id. Every
 * one of those steps is generic code shared with three other families, which
 * is exactly why a new family can break on it without anything else noticing.
 */
func TestLivePulsarConnectsThroughTheConnectionService(t *testing.T) {
	connections, registry := livePulsarStack(t)

	// The "test connection" button, which probes a profile nobody has saved.
	if err := connections.ProbeProfile(livePulsarProfile("probe")); err != nil {
		t.Fatalf("probe an unsaved pulsar profile: %v", err)
	}

	profile, err := connections.AddConnection(livePulsarProfile("pulsar-live"))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := connections.Connect(profile.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}

	stored, err := connections.GetConnection(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.StatusOnline {
		t.Fatalf("stored status = %q, want online", stored.Status)
	}

	// The boards resolve a connection out of the registry by id, so a connect
	// that stored "online" without leaving one there is a connection every
	// page reports as offline.
	conn, ok := registry.Get(profile.ID)
	if !ok {
		t.Fatal("connecting stored no connection in the registry")
	}
	if conn.Kind() != model.KindPulsar {
		t.Fatalf("registry holds a %q connection", conn.Kind())
	}

	if err := connections.Disconnect(profile.ID); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if _, open := registry.Get(profile.ID); open {
		t.Error("disconnecting left the connection open")
	}
}

/*
 * The test button refuses a profile whose admin address is wrong.
 *
 * Pulsar is the first family whose two addresses are collected separately, so
 * it is the first where one of them can be wrong on its own - a broker port
 * that answers beside an admin API that does not. The dialog's test button is
 * where that has to surface, because Connect deliberately does not fail on it:
 * it opens the connection and lets the capability probe explain what is
 * missing, the same as every other family.
 */
func TestLivePulsarTestButtonRefusesAWrongAdminAddress(t *testing.T) {
	connections, _ := livePulsarStack(t)

	broken := livePulsarProfile("pulsar-wrong-admin")
	// Port 1 is reserved and never bound, so the broker port is live and the
	// admin API is not.
	broken.Options[pulsar.OptionAdminURL] = "http://127.0.0.1:1"

	if err := connections.ProbeProfile(broken); err == nil {
		t.Fatal("probing a profile with an unreachable admin API succeeded")
	}

	// And the mirror: the admin API is live and the broker port is not. Both
	// halves have to be checked, or "test connection" passes on a profile that
	// can read every page and publish nothing.
	halfOpen := livePulsarProfile("pulsar-wrong-broker")
	halfOpen.Endpoints = "pulsar://127.0.0.1:1"

	if err := connections.ProbeProfile(halfOpen); err == nil {
		t.Fatal("probing a profile with an unreachable broker port succeeded")
	}
}
