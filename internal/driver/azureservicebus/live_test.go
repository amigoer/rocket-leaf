package azureservicebus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * The live environment, as tests/e2e/azure-servicebus/compose.yaml publishes it.
 *
 * Microsoft's own emulator, two ports and two containers. The endpoint is its
 * AMQP port, reached through the same UseDevelopmentEmulator flag a developer
 * would set by hand; the management host is the connection form's own option,
 * so both are exercised by every test here rather than by none.
 *
 * What this environment cannot do is asserted rather than stepped around, and
 * there are four such gaps. They are named on the tests that pin them:
 *
 *   - no message counts anywhere (TestLiveCountsAreDegradedAgainstTheEmulator),
 *   - no topic in the startup config with zero subscriptions, which is why the
 *     seed creates that one through the management API,
 *   - ForwardDeadLetteredMessagesTo wants an absolute URI rather than an
 *     entity name (TestLiveForwardingWantsAnAbsoluteURI),
 *   - and the namespace name is the emulator's own, not one this app chose.
 */
const (
	liveNamespace  = "localhost:5672"
	liveManagement = "127.0.0.1:5300"
	liveKeyName    = "RootManageSharedAccessKey"
	liveKey        = "SAS_KEY_VALUE"
)

// The entities tests/e2e/azure-servicebus/config.json declares and
// scripts/e2e-azure-servicebus-seed.sh fills. Tests read these and never write
// to them; anything a test needs to change it creates for itself.
const (
	seedOrders   = "mqs-seed-orders"
	seedFailures = "mqs-seed-failures"
	seedQuiet    = "mqs-seed-quiet"
	seedEvents   = "mqs-seed-events"
	seedOrphaned = "mqs-seed-orphaned"

	seedSubAll    = "mqs-seed-events-all"
	seedSubRed    = "mqs-seed-events-red"
	seedSubOrders = "mqs-seed-events-orders"
)

func requireNamespace(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.AzureServiceBus,
		Name:   "the azure service bus emulator",
		Start:  "npm run e2e:azure-servicebus:up",
		// /health answers only once the emulator has created every entity its
		// config names, so this is the state the tests need rather than a
		// socket that happens to be open.
		Probe: e2e.HTTPGet("http://" + liveManagement + "/health"),
	})
}

// liveProfile is the environment as a user would configure it: a namespace in
// the endpoint field, a shared access key, and the emulator's management port
// beside them.
func liveProfile() model.ConnectionProfile {
	profile := model.ConnectionProfile{
		ID:         1,
		Name:       "azure service bus e2e",
		Kind:       model.KindAzureServiceBus,
		Endpoints:  liveNamespace,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options: map[string]string{
			OptionSharedAccessKeyName: liveKeyName,
			OptionEmulatorManagement:  liveManagement,
		},
	}
	profile.SetSecret(SecretSharedAccessKey, liveKey)
	return profile
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireNamespace(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

/*
 * The point of this family's connection shape, exercised end to end.
 *
 * This is the first hosted family whose profile carries an address, and the
 * descriptor test only says the form asks for one. This proves the driver
 * needs it: the namespace in Endpoints is what both clients are built from,
 * and a profile without it opens nothing.
 */
func TestLiveOpenDialsTheNamespaceInTheProfile(t *testing.T) {
	requireNamespace(t)

	profile := liveProfile()
	if profile.Endpoints == "" {
		t.Fatal("the live profile carries no address, and this family is reached by one")
	}

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn.Namespace() != liveNamespace {
		t.Errorf("namespace = %q, want %q", conn.Namespace(), liveNamespace)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}

	profile.Endpoints = ""
	if _, err := open(liveContext(t), profile); err == nil {
		t.Error("a profile naming no namespace opened anyway")
	}
}

/*
 * Neither client dials on construction, so a namespace that does not exist, a
 * key for the wrong one and a rotated key all look identical until a request
 * is made. Open has to make one, or every one of them opens and then reports
 * an empty namespace.
 */
func TestLiveOpenProvesTheCredentialReachesTheNamespace(t *testing.T) {
	requireNamespace(t)

	t.Run("a management host with nothing behind it", func(t *testing.T) {
		profile := liveProfile()
		profile.Options[OptionEmulatorManagement] = "127.0.0.1:1"
		profile.TimeoutSec = 5

		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened against a port with nothing listening")
		}
	})

	t.Run("no credential at all", func(t *testing.T) {
		profile := liveProfile()
		profile.SetSecret(SecretSharedAccessKey, "")

		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened with no credential")
		}
		// The message has to name the missing field: this family has no
		// ambient credential to have fallen back on, unlike the two hosted
		// families before it.
		if !strings.Contains(err.Error(), "credential") {
			t.Errorf("the refusal does not mention the credential: %v", err)
		}
	})
}

// A connection string pasted whole is the other way to fill this form, and it
// has to reach the same namespace as its parts.
func TestLiveOpenTakesAPastedConnectionString(t *testing.T) {
	requireNamespace(t)

	profile := model.ConnectionProfile{
		ID:         2,
		Name:       "azure service bus e2e, pasted",
		Kind:       model.KindAzureServiceBus,
		Endpoints:  liveNamespace,
		TimeoutSec: 20,
		Options:    map[string]string{OptionEmulatorManagement: liveManagement},
	}
	profile.SetSecret(SecretConnectionString,
		"Endpoint=sb://"+liveNamespace+";SharedAccessKeyName="+liveKeyName+";SharedAccessKey="+liveKey)

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// Close has to tolerate a second call: the registry closes on disconnect and
// again on shutdown.
func TestLiveCloseIsIdempotent(t *testing.T) {
	conn := liveConn(t)

	if err := conn.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection answered a ping")
	}
}

// The emulator is what this suite runs against, and the driver knows it. The
// capability narrowing below depends on that being true rather than assumed.
func TestLiveConnectionKnowsItIsAnEmulator(t *testing.T) {
	conn := liveConn(t)

	if conn.Emulator() != liveManagement {
		t.Errorf("emulator = %q, want %q", conn.Emulator(), liveManagement)
	}
	if !strings.Contains(conn.endpointName(), "emulator") {
		t.Errorf("errors here would call the endpoint %q", conn.endpointName())
	}
}
