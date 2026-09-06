package googlepubsub

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live environment, as tests/e2e/google-pubsub/compose.yaml publishes it.
//
// Google's own emulator, reached through the emulator-host option. That is not
// a testing shortcut around the real code path: the option is the connection
// form's own field, so every test here exercises it.
const (
	liveEmulator = "127.0.0.1:8085"
	liveProject  = "mq-studio-e2e"
)

// The objects scripts/e2e-google-pubsub-seed.sh creates. Tests read these and
// never write to them; anything a test needs to change it creates for itself.
const (
	seedOrders      = "mqs-seed-orders"
	seedDeadLetters = "mqs-seed-dead-letters"
	seedOrphaned    = "mqs-seed-orphaned"
	seedQuiet       = "mqs-seed-quiet"

	seedWorker     = "mqs-seed-orders-worker"
	seedAudit      = "mqs-seed-orders-audit"
	seedDeadReader = "mqs-seed-dead-letters-reader"
	seedIdle       = "mqs-seed-quiet-idle"
)

func requireProject(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.GooglePubSub,
		Name:   "the google pub/sub emulator",
		Start:  "npm run e2e:google-pubsub:up",
		// The emulator serves the same REST surface as the real API, so this
		// is a call rather than a socket that happens to be open.
		Probe: e2e.HTTPGet("http://" + liveEmulator + "/v1/projects/" + liveProject + "/topics"),
	})
}

// liveProfile is the environment as a user would configure it: a project, an
// emulator host, and no address anywhere on the form.
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "google pub/sub e2e",
		Kind:       model.KindGooglePubSub,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthNone},
		Options: map[string]string{
			OptionProjectID:    liveProject,
			OptionEmulatorHost: liveEmulator,
		},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireProject(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

/*
 * The point of this family, exercised end to end.
 *
 * A profile whose Endpoints is empty opens, which only one other family here
 * can do. It is asserted rather than only checked in the descriptor test
 * because the descriptor only says the form asks for no address - this proves
 * the driver needs none either.
 */
func TestLiveOpenNeedsNoAddress(t *testing.T) {
	requireProject(t)

	profile := liveProfile()
	if profile.Endpoints != "" {
		t.Fatalf("the live profile carries an address %q; this family has none", profile.Endpoints)
	}

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn.Project() != liveProject {
		t.Errorf("project = %q, want %q", conn.Project(), liveProject)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// Nothing is dialled, so a wrong project, a missing credential and an
// unreachable endpoint all look identical until a request is made. Open has to
// make one, or every one of them opens and then reports an empty project.
func TestLiveOpenProvesTheCredentialReachesTheProject(t *testing.T) {
	requireProject(t)

	t.Run("an emulator host with nothing behind it", func(t *testing.T) {
		profile := liveProfile()
		profile.TimeoutSec = 2
		profile.Options[OptionEmulatorHost] = "127.0.0.1:8099"
		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened against a host that answers nothing")
		}
	})

	t.Run("a profile naming no project", func(t *testing.T) {
		profile := liveProfile()
		delete(profile.Options, OptionProjectID)
		_, err := open(liveContext(t), profile)
		if err == nil {
			t.Fatal("opened with no project to name resources in")
		}
		if !strings.Contains(err.Error(), "project") {
			t.Errorf("error does not name the missing field: %v", err)
		}
	})
}

/*
 * What the emulator is not, recorded rather than worked around.
 *
 * A project that does not exist answers an empty listing here, where the real
 * service answers NOT_FOUND or PERMISSION_DENIED: the emulator keeps no
 * registry of projects and treats any name as one it has simply never seen a
 * topic for. So the ping below succeeds against a project nobody created, and
 * that is the emulator rather than the driver.
 *
 * It is asserted rather than skipped because the opposite would be worse: a
 * driver that started refusing an unknown project would fail here and nothing
 * would say why.
 */
func TestLiveEmulatorAcceptsAnyProjectName(t *testing.T) {
	requireProject(t)

	profile := liveProfile()
	profile.Options[OptionProjectID] = "mqs-test-no-such-project"
	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("the emulator refused an unknown project, which the real service does "+
			"and this one did not use to: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
}

// Close is called on disconnect and again on shutdown, so the second call has
// to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireProject(t)

	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answers a ping")
	}
}
