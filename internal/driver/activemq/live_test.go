package activemq

import (
	"context"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The two live brokers, and why there are two.
//
// Neither is a degraded environment: they are the family's two products, and
// they agree on nothing this driver reads. A test green against Artemis says
// nothing about Classic, so every behaviour below runs against both.
const (
	liveArtemisConsole = "http://127.0.0.1:8161"
	liveArtemisUser    = "artemis"
	liveArtemisPass    = "artemis"
	liveArtemisAMQP    = "amqp://127.0.0.1:61616"

	liveClassicConsole = "http://127.0.0.1:8162"
	liveClassicUser    = "admin"
	liveClassicPass    = "admin"
	liveClassicAMQP    = "amqp://127.0.0.1:5673"
)

func requireArtemis(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the artemis broker",
		Start:  "npm run e2e:activemq:up",
		// A Jolokia search rather than a GET on the console: Jetty binds 8161
		// while the broker is still starting, so the console answering proves
		// only that the web server is up.
		Probe: e2e.HTTPGet(liveArtemisConsole + "/console/jolokia/search/org.apache.activemq.artemis:broker=*"),
	})
}

func requireClassic(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the activemq classic broker",
		Start:  "npm run e2e:activemq:classic:up",
		Probe:  e2e.HTTPGet(liveClassicConsole + "/api/jolokia/search/org.apache.activemq:type=Broker,brokerName=*"),
	})
}

// artemisProfile and classicProfile are the two endpoints as a user would
// configure them - console address, console credentials, and the AMQP
// acceptor when the tier is wanted.
func artemisProfile(amqp string) model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         1,
		Name:       "artemis e2e",
		Kind:       model.KindActiveMQ,
		Endpoints:  liveArtemisConsole,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{OptionAMQPURL: amqp},
		Secrets: map[string]string{
			SecretUsername: liveArtemisUser,
			SecretPassword: liveArtemisPass,
		},
	}
}

func classicProfile(amqp string) model.ConnectionProfile {
	return model.ConnectionProfile{
		ID:         2,
		Name:       "classic e2e",
		Kind:       model.KindActiveMQ,
		Endpoints:  liveClassicConsole,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{OptionAMQPURL: amqp},
		Secrets: map[string]string{
			SecretUsername: liveClassicUser,
			SecretPassword: liveClassicPass,
		},
	}
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// The probe is the driver's first decision and every read afterwards branches
// on it, so getting it wrong is not a wrong answer on one page - it is every
// page addressing MBeans that do not exist.
func TestLiveProbeTellsTheTwoProductsApart(t *testing.T) {
	t.Run("artemis", func(t *testing.T) {
		requireArtemis(t)
		conn, err := open(liveContext(t), artemisProfile(""))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if conn.tiers.product != artemis {
			t.Errorf("product = %q, want artemis", conn.tiers.product)
		}
		if conn.names.broker != "0.0.0.0" {
			t.Errorf("broker = %q, want 0.0.0.0", conn.names.broker)
		}
	})

	t.Run("classic", func(t *testing.T) {
		requireClassic(t)
		conn, err := open(liveContext(t), classicProfile(""))
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = conn.Close() }()

		if conn.tiers.product != classic {
			t.Errorf("product = %q, want classic", conn.tiers.product)
		}
		if conn.names.broker != "localhost" {
			t.Errorf("broker = %q, want localhost", conn.names.broker)
		}
	})
}

// Ping reads an attribute off the broker MBean rather than fetching the
// console, because Jetty answers on 8161 both before the broker has started
// and after it has stopped.
func TestLivePingAsksTheBrokerRatherThanTheConsole(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		profile model.ConnectionProfile
	}{
		{"artemis", requireArtemis, artemisProfile("")},
		{"classic", requireClassic, classicProfile("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)
			conn, err := open(ctx, tc.profile)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = conn.Close() }()

			if err := conn.Ping(ctx); err != nil {
				t.Errorf("ping: %v", err)
			}
			_ = conn.Close()
			if err := conn.Ping(ctx); err == nil {
				t.Error("a closed connection still answered a ping")
			}
		})
	}
}

// The three states of the optional tier, each of which sends a user somewhere
// different: to the connection form, to the broker's acceptor list, or
// nowhere because it is working.
func TestLiveAMQPTierReportsWhichOfItsStatesItIsIn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		require  func(*testing.T)
		profile  func(string) model.ConnectionProfile
		acceptor string
	}{
		{"artemis", requireArtemis, artemisProfile, liveArtemisAMQP},
		{"classic", requireClassic, classicProfile, liveClassicAMQP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)

			live, err := open(ctx, tc.profile(tc.acceptor))
			if err != nil {
				t.Fatalf("open with the acceptor: %v", err)
			}
			defer func() { _ = live.Close() }()
			if live.tiers.amqpReason != "" {
				t.Errorf("the acceptor is open and the tier reported %q", live.tiers.amqpReason)
			}

			unset, err := open(ctx, tc.profile(""))
			if err != nil {
				t.Fatalf("open with no acceptor configured: %v", err)
			}
			defer func() { _ = unset.Close() }()
			if unset.tiers.amqpReason != amqpAbsent {
				t.Errorf("reason with no address = %q, want %q", unset.tiers.amqpReason, amqpAbsent)
			}

			// A port nothing is listening on. Told apart from the above
			// because "you did not configure it" and "the broker refused"
			// send a user to two different places.
			closed, err := open(ctx, tc.profile("amqp://127.0.0.1:1"))
			if err != nil {
				t.Fatalf("open with a closed acceptor: %v", err)
			}
			defer func() { _ = closed.Close() }()
			if closed.tiers.amqpReason != amqpUnreachable {
				t.Errorf("reason for a closed port = %q, want %q", closed.tiers.amqpReason, amqpUnreachable)
			}
		})
	}
}

// Both brokers ship jolokia-access.xml with strict-checking, so a request
// carrying no Origin is refused as coming from the null origin - and the
// refusal is a 403 that reads exactly like bad credentials. This asserts the
// header is what makes the difference, against the real policy file rather
// than against a fixture repeating what the driver believes.
func TestLiveJolokiaRefusesACallWithNoOrigin(t *testing.T) {
	for _, tc := range []struct {
		name    string
		require func(*testing.T)
		console string
		path    string
		user    string
		pass    string
		pattern string
	}{
		{"artemis", requireArtemis, liveArtemisConsole, artemisPath, liveArtemisUser, liveArtemisPass,
			artemisDomain + ":broker=*"},
		{"classic", requireClassic, liveClassicConsole, classicPath, liveClassicUser, liveClassicPass,
			classicDomain + ":type=Broker,brokerName=*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.require(t)
			ctx := liveContext(t)

			with, err := newJolokiaClient(tc.console, tc.path, tc.user, tc.pass, "", 10*time.Second, false)
			if err != nil {
				t.Fatalf("client: %v", err)
			}
			found, err := with.search(ctx, tc.pattern)
			if err != nil {
				t.Fatalf("search with an origin: %v", err)
			}
			if len(found) == 0 {
				t.Fatal("search with an origin found no broker")
			}

			// A client built without one, which is the only way to send no
			// header at all - setting the field to "" on a constructed client
			// sends an empty Origin, which is a different thing to the agent.
			without := *with
			without.origin = ""
			if _, err := without.search(ctx, tc.pattern); err == nil {
				// Artemis skips here and Classic does not, and the difference
				// is the image rather than the product: apache/activemq-artemis
				// defaults EXTRA_ARGS to --relax-jolokia, which strips
				// <strict-checking/> out of the generated policy. An Artemis
				// created without that flag refuses exactly as Classic does,
				// so the header stays mandatory in the driver.
				t.Skip("this broker's jolokia-access.xml does not check the origin")
			} else if !forbidden(err) {
				t.Errorf("a call with no origin failed with %v, want a refusal", err)
			}
		})
	}
}

// Credentials that are wrong have to fail as credentials. The agent answers a
// refusal the same way it answers an origin it does not like, so a driver that
// retried past it would report "no broker here" for a typo in a password.
func TestLiveWrongCredentialsFailAsARefusal(t *testing.T) {
	requireArtemis(t)
	profile := artemisProfile("")
	profile.Secrets[SecretPassword] = "not-the-password"

	if _, err := open(liveContext(t), profile); err == nil {
		t.Fatal("opened with a wrong password")
	}
}
