package ibmmq

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live queue manager, and the two accounts the developer image ships.
//
// They are two because the mqweb server maps its two roles to two groups and
// the image puts one user in each: admin holds MQWebAdmin and reaches the
// administrative interface, app holds MQWebUser and reaches the messaging one.
// Neither can do the other's work, which is what makes this environment able
// to exercise both halves of the driver's tier split rather than only the
// happy path.
const (
	liveMQWeb        = "https://127.0.0.1:9443"
	liveQueueManager = "QM1"

	liveAdminUser = "admin"
	liveAdminPass = "passw0rd"
	liveAppUser   = "app"
	liveAppPass   = "passw0rd"
)

// Objects the seed made, which the live tests read and never change. Anything
// a test creates is named MQS.TEST.* so the two can never collide.
const (
	seedQueue        = "MQS.SEED.ORDERS"
	seedAuditQueue   = "MQS.SEED.AUDIT"
	seedBackoutQueue = "MQS.SEED.BACKOUT"
	seedSubQueue     = "MQS.SEED.SUBQ"
	seedTopic        = "MQS.SEED.EVENTS"
	seedTopicString  = "dev/seed/events"
	seedSubscription = "MQS.SEED.SUB"
	seedChannel      = "MQS.SEED.SDR"
	deadLetterQueue  = "DEV.DEAD.LETTER.QUEUE"
)

func requireIBMMQ(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.IBMMQ,
		Name:   "the ibm mq queue manager",
		Start:  "npm run e2e:ibmmq:up",
		Probe:  probeQueueManager,
	})
}

/*
 * probeQueueManager asks the REST API whether the queue manager is running.
 *
 * Not e2e.HTTPGet and not e2e.DialTCP, for two reasons that both matter.
 * The shared HTTP probe verifies certificates and the mqweb server presents
 * one it signed itself, so it would report every healthy environment as
 * absent. A TCP dial would go the other way and report an unhealthy one as
 * present: Liberty binds 9443 and serves the console while the queue manager
 * is still starting, which is the window this whole environment exists to
 * never hand a test.
 */
func probeQueueManager() error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // a self-signed development certificate
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}

	request, err := http.NewRequest(http.MethodGet, liveMQWeb+"/ibmmq/rest/v1/admin/qmgr/"+liveQueueManager, nil)
	if err != nil {
		return err
	}
	request.SetBasicAuth(liveAdminUser, liveAdminPass)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return &restError{Status: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	}
	return nil
}

/*
 * liveProfile is the environment as a user would configure it.
 *
 * Skip-verify is on, and it is on in the profile rather than anywhere shared:
 * the mqweb server generated its own certificate, and the switch that accepts
 * it is a decision this profile makes. A test that turned verification off in
 * the driver, in the HTTP client or in an environment variable would be
 * turning it off for every user, which is the one thing this option must never
 * do - and TestLiveVerificationIsOffOnlyWhenTheProfileAsks below is what pins
 * that.
 */
func liveProfile() model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "ibm mq e2e",
		Kind:       model.KindIBMMQ,
		Endpoints:  liveMQWeb,
		TimeoutSec: 20,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			OptionQueueManager:  liveQueueManager,
			OptionTLSSkipVerify: "true",
		},
		Secrets: map[string]string{
			SecretUsername:          liveAdminUser,
			SecretPassword:          liveAdminPass,
			SecretMessagingUsername: liveAppUser,
			SecretMessagingPassword: liveAppPass,
		},
	}
}

// adminOnlyProfile holds the administrative role and not the messaging one,
// which is what the developer image's admin account actually is.
func adminOnlyProfile() model.ConnectionProfile {
	profile := liveProfile()
	profile.Secrets = map[string]string{
		SecretUsername: liveAdminUser,
		SecretPassword: liveAdminPass,
		// Named explicitly rather than left to the fallback: the point of this
		// profile is a messaging credential that authenticates and holds the
		// wrong role, and an empty pair would reuse the administrative one and
		// arrive at the same place by accident.
		SecretMessagingUsername: liveAdminUser,
		SecretMessagingPassword: liveAdminPass,
	}
	return profile
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func liveConn(t *testing.T) *Conn {
	t.Helper()
	requireIBMMQ(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestLiveOpenReachesTheQueueManager(t *testing.T) {
	conn := liveConn(t)

	if conn.Kind() != model.KindIBMMQ {
		t.Errorf("kind = %q, want ibmmq", conn.Kind())
	}
	if conn.QueueManager() != liveQueueManager {
		t.Errorf("queue manager = %q, want %q", conn.QueueManager(), liveQueueManager)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

/*
 * A profile that names no queue manager gets the one the server fronts.
 *
 * This is the ordinary case - most installations run one - and it is what
 * keeps the field optional on the form. It is also the half that has to be
 * proved against a real server: the listing is what supplies the name, and a
 * driver that defaulted to something would work here and address the wrong
 * queue manager on an installation with two.
 */
func TestLiveOpenDiscoversTheOnlyQueueManager(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionQueueManager] = ""

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if conn.QueueManager() != liveQueueManager {
		t.Errorf("discovered %q, want %q", conn.QueueManager(), liveQueueManager)
	}
}

// A queue manager that is not there fails at open, where the message can still
// name the field and list what the server does front. Discovering it at the
// first board instead would report every page as broken.
func TestLiveOpenRefusesAQueueManagerThatIsNotThere(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionQueueManager] = "NOSUCHQM"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection to a queue manager that does not exist")
	}
	if !strings.Contains(err.Error(), liveQueueManager) {
		t.Errorf("error does not say which queue managers are there: %v", err)
	}
}

/*
 * The messaging interface is a tier of its own, and the probe is what settles
 * whether this connection has it.
 *
 * Both halves are asserted here because the environment can supply both, which
 * is unusual: the developer image ships one account per mqweb role, so the same
 * server answers "yes" to one credential and "the role is not mapped" to
 * another. A driver that assumed one credential reaches both interfaces would
 * pass every other test in this file.
 */
func TestLiveMessagingTierIsProbedSeparately(t *testing.T) {
	requireIBMMQ(t)

	t.Run("with the messaging account", func(t *testing.T) {
		conn, err := open(liveContext(t), liveProfile())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if reason := conn.probeMessaging(liveContext(t)); reason != "" {
			t.Errorf("messaging reported unavailable as %s, and the app account holds MQWebUser", reason)
		}
	})

	t.Run("with the administrative account only", func(t *testing.T) {
		conn, err := open(liveContext(t), adminOnlyProfile())
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		reason := conn.probeMessaging(liveContext(t))
		if reason != messagingForbidden {
			t.Errorf("messaging reason = %q, want %q; the admin account is not in MQWebMessaging",
				reason, messagingForbidden)
		}
	})

	t.Run("with a password the server will not take", func(t *testing.T) {
		profile := liveProfile()
		profile.Secrets[SecretMessagingPassword] = "not-the-password"

		conn, err := open(liveContext(t), profile)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		reason := conn.probeMessaging(liveContext(t))
		if reason != messagingRefused {
			t.Errorf("messaging reason = %q, want %q; a rejected credential and an unmapped "+
				"role send a reader to different places", reason, messagingRefused)
		}
	})
}

// An administrative credential the server will not take fails at open. It has
// to: every board reads through that interface, and a connection that opened
// anyway would report an empty queue manager rather than a refused login.
func TestLiveOpenRefusesAnAdministrativeCredentialTheServerRejects(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Secrets[SecretPassword] = "not-the-password"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("opened a connection with a password the mqweb server rejects")
	}
}

/*
 * Verification is off only because this profile asked, and this is the test
 * that keeps it that way.
 *
 * The mqweb server signs its own certificate, so every test in this file needs
 * the switch - which is exactly the situation in which somebody quietly turns
 * verification off in the driver, or in a shared HTTP client, and nobody
 * notices for a release. The same profile with the switch off must fail, and
 * fail on the certificate rather than on anything else.
 */
func TestLiveVerificationIsOffOnlyWhenTheProfileAsks(t *testing.T) {
	requireIBMMQ(t)
	profile := liveProfile()
	profile.Options[OptionTLSSkipVerify] = "false"

	conn, err := open(liveContext(t), profile)
	if err == nil {
		_ = conn.Close()
		t.Fatal("connected to a self-signed mqweb server with verification on")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "certificate") {
		t.Errorf("failed for a reason other than the certificate: %v", err)
	}
}

// The queue manager's state is what Ping reads, and a closed connection stops
// answering. The registry closes on disconnect and on shutdown, so the second
// close has to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireIBMMQ(t)
	conn, err := open(liveContext(t), liveProfile())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := conn.Ping(liveContext(t)); err == nil {
		t.Error("a closed connection still answers a ping")
	}
}
