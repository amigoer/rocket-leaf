package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	activemqdriver "github.com/amigoer/mq-studio/internal/driver/activemq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	activemqservice "github.com/amigoer/mq-studio/internal/service/activemq"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/message"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The ActiveMQ stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * credential that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * ActiveMQ is the third family with two independent credentials - the console
 * the pages read through, and the AMQP acceptor the watch page attaches to -
 * and losing the second is nearly invisible: everything still connects, every
 * board still fills, and only following a topic quietly stops being offered.
 */

const (
	liveAMQConsole  = "http://127.0.0.1:8161"
	liveAMQProbe    = liveAMQConsole + "/console/jolokia/search/org.apache.activemq.artemis:broker=*"
	liveAMQUser     = "artemis"
	liveAMQPassword = "artemis"
	liveAMQAcceptor = "amqp://127.0.0.1:61616"
	// A second account for the acceptor, so a test can tell "the console's
	// credentials were reused" apart from "the AMQP pair survived disk".
	liveAMQAcceptorUser = "artemis"
	liveAMQAcceptorPass = "artemis"
)

func requireLiveActiveMQ(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the artemis e2e broker",
		Start:  "npm run e2e:activemq:up",
		// A Jolokia search rather than a GET on the console: Jetty binds 8161
		// while the broker is still starting, so the console answering proves
		// only that the web server is up.
		Probe: e2e.HTTPGet(liveAMQProbe),
	})
}

// activeMQStack is the connection service, the ActiveMQ service and the
// canonical services a board reads through, on a config directory of its own.
type activeMQStack struct {
	connections  *connection.Service
	activemq     *activemqservice.Service
	destinations *destination.Service
	messages     *message.Service
	consumers    *subscription.Service
}

func newActiveMQStack(t *testing.T) *activeMQStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindActiveMQ); !ok {
		driver.Register(activemqdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &activeMQStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		activemq:     activemqservice.New(conns, settingsService),
		destinations: destination.New(conns, settingsService),
		messages:     message.New(conns, settingsService),
		consumers:    subscription.New(conns, settingsService),
	}
}

// liveActiveMQProfile is the broker as a user would configure it, with or
// without the optional acceptor.
func liveActiveMQProfile(name string, withAcceptor bool) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindActiveMQ,
		Endpoints:  liveAMQConsole,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{},
		Secrets: map[string]string{
			activemqdriver.SecretUsername: liveAMQUser,
			activemqdriver.SecretPassword: liveAMQPassword,
		},
	}
	if withAcceptor {
		profile.Options[activemqdriver.OptionAMQPURL] = liveAMQAcceptor
		profile.Secrets[activemqdriver.SecretAMQPUsername] = liveAMQAcceptorUser
		profile.Secrets[activemqdriver.SecretAMQPPassword] = liveAMQAcceptorPass
	}
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *activeMQStack) dial(t *testing.T, profile model.ConnectionProfile) int {
	t.Helper()
	created, err := s.connections.AddConnection(profile)
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}
	if err := s.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = s.connections.Disconnect(created.ID) })
	return created.ID
}

/*
 * The whole path in one go: store a profile, dial it, then declare a
 * destination, send to it and browse the message back through the id a page
 * would pass.
 *
 * Writing and reading on the one connection is the point rather than a
 * shortcut. It is what the destinations board, the send console and the
 * message browser are - three pages on one profile - and the write goes
 * through the ActiveMQ service while the reads go through the canonical ones,
 * which is exactly the split the boards use.
 */
func TestLiveActiveMQThroughAConnectionID(t *testing.T) {
	requireLiveActiveMQ(t)
	stack := newActiveMQStack(t)
	connID := stack.dial(t, liveActiveMQProfile("activemq-live", true))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	queue := "MQS.APP.LIVE"
	_ = stack.activemq.RemoveDestination(ctx, connID, queue)
	if err := stack.activemq.CreateDestination(ctx, connID, queue, false); err != nil {
		t.Fatalf("CreateDestination: %v", err)
	}
	t.Cleanup(func() { _ = stack.activemq.RemoveDestination(context.Background(), connID, queue) })

	result, err := stack.activemq.Publish(ctx, connID, model.PublishRequest{
		RoutingKey: queue,
		Body:       "through-the-service",
		Persistent: true,
		Count:      1,
		Headers:    map[string]string{"tenant": "acme"},
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Sent != 1 {
		t.Fatalf("sent = %d, want 1 (%s)", result.Sent, result.Reason)
	}

	// Read through the canonical service, which is what the message board
	// calls - so this covers the capability check and the type assertion the
	// driver's own tests bypass.
	messages, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{Topic: queue})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("browse returned %d messages, want 1", len(messages))
	}
	if got := messages[0].Body; got != "through-the-service" {
		t.Errorf("body = %q", got)
	}
	if got := messages[0].Properties["tenant"]; got != "acme" {
		t.Errorf("the header set through the service came back as %q", got)
	}

	// And the destination reads back through the canonical listing, which is
	// what the destinations board calls.
	destinations, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, entry := range destinations {
		if entry.Ref.Name == queue {
			found = true
			if entry.Depth != 1 {
				t.Errorf("depth = %d, want 1", entry.Depth)
			}
		}
	}
	if !found {
		t.Error("the destination created through the service is not in the canonical listing")
	}
}

/*
 * The credentials, through disk.
 *
 * Everything above could pass against a service that never wrote the secrets:
 * the profile it dialled is the one held in memory. Reading them back is the
 * assertion, and it is the one this project has actually got wrong before -
 * a connection form's test button probes the profile it was handed, not the
 * one that was stored, so it proves nothing about either.
 */
func TestLiveActiveMQCredentialsSurviveDisk(t *testing.T) {
	requireLiveActiveMQ(t)
	stack := newActiveMQStack(t)

	created, err := stack.connections.AddConnection(liveActiveMQProfile("activemq-credentials", true))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	stored, err := stack.connections.GetConnection(created.ID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	for _, pair := range []struct{ key, want string }{
		{activemqdriver.SecretUsername, liveAMQUser},
		{activemqdriver.SecretPassword, liveAMQPassword},
		{activemqdriver.SecretAMQPUsername, liveAMQAcceptorUser},
		{activemqdriver.SecretAMQPPassword, liveAMQAcceptorPass},
	} {
		if got := stored.Secret(pair.key); got != pair.want {
			t.Errorf("%s did not survive being stored: %q", pair.key, got)
		}
	}
	// The mechanism is what the old bug reset to none, and a profile that kept
	// its passwords and lost its mechanism authenticates as nobody.
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism = %q, want plain", stored.Auth.Mechanism)
	}
	if stored.Option(activemqdriver.OptionAMQPURL) != liveAMQAcceptor {
		t.Error("the acceptor address did not survive being stored")
	}

	if err := stack.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = stack.connections.Disconnect(created.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The proof that the stored AMQP credential is the one that was used.
	// Nothing above needs it: every board fills through the console, and this
	// is the only operation that opens the acceptor at all - which is exactly
	// why losing that half is invisible without a test for it.
	topic := "MQS.APP.WATCH"
	_ = stack.activemq.RemoveDestination(ctx, created.ID, topic)
	if err := stack.activemq.CreateDestination(ctx, created.ID, topic, true); err != nil {
		t.Fatalf("create the topic to watch: %v", err)
	}
	t.Cleanup(func() { _ = stack.activemq.RemoveDestination(context.Background(), created.ID, topic) })

	stream, err := stack.activemq.StartSubscription(ctx, created.ID, model.LiveSubscriptionSpec{
		Filters: []model.LiveFilter{{Pattern: topic}},
	})
	if err != nil {
		t.Fatalf("StartSubscription through the stored credential: %v", err)
	}
	t.Cleanup(func() { _ = stack.activemq.StopSubscription(context.Background(), created.ID, stream.ID) })
	if !stream.Live {
		t.Error("the stream opened and reports itself as not live")
	}
}

/*
 * A profile with no acceptor configured still works, and says what it lost.
 *
 * The optional tier's whole point: every board fills, and the one capability
 * it cannot answer is degraded with a reason rather than missing.
 */
func TestLiveActiveMQWithoutTheAcceptorKeepsEveryOtherPage(t *testing.T) {
	requireLiveActiveMQ(t)
	stack := newActiveMQStack(t)
	connID := stack.dial(t, liveActiveMQProfile("activemq-no-acceptor", false))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := stack.destinations.List(ctx, connID, model.DestinationFilter{}); err != nil {
		t.Errorf("the destinations board cannot fill without the acceptor: %v", err)
	}
	if _, err := stack.consumers.List(ctx, connID); err != nil {
		t.Errorf("the subscriptions board cannot fill without the acceptor: %v", err)
	}

	// And the one thing it cannot do reports itself rather than failing
	// somewhere unhelpful.
	_, err := stack.activemq.StartSubscription(ctx, connID, model.LiveSubscriptionSpec{
		Filters: []model.LiveFilter{{Pattern: "MQS.SEED.events"}},
	})
	if err == nil {
		t.Fatal("following was offered with no acceptor configured")
	}
	if !strings.Contains(err.Error(), "amqp") {
		t.Errorf("the reason does not name the tier that is missing: %v", err)
	}
}
