package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/crypto"
	"github.com/amigoer/mq-studio/internal/driver"
	natsdriver "github.com/amigoer/mq-studio/internal/driver/nats"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
	"github.com/amigoer/mq-studio/internal/service/connection"
	"github.com/amigoer/mq-studio/internal/service/destination"
	"github.com/amigoer/mq-studio/internal/service/message"
	natsservice "github.com/amigoer/mq-studio/internal/service/nats"
	"github.com/amigoer/mq-studio/internal/service/settings"
	"github.com/amigoer/mq-studio/internal/service/subscription"
	"github.com/amigoer/mq-studio/internal/storage/layout"
)

/*
 * The NATS stack from the outside, through a connection id.
 *
 * The driver's own live tests hold a *Conn. These hold nothing: they store a
 * profile, dial it, and then ask the service layer the way a page does - with
 * an integer. What that covers and the driver's tests do not is the chain in
 * between, where the two failures this project has actually had both lived: a
 * credential that did not survive being written to disk and read back, and a
 * capability the service checks before the type assertion.
 *
 * NATS is the second family with two independent credentials - the account the
 * pages read through, and the system account that answers for the cluster - so
 * it is the second that can lose one and keep the other, and the one where
 * losing the second is nearly invisible: everything still connects, and only
 * the cluster pages quietly narrow to one server.
 */

const (
	liveNatsServers    = "nats://127.0.0.1:4222,nats://127.0.0.1:4223,nats://127.0.0.1:4224"
	liveNatsMonitor    = "http://127.0.0.1:8222"
	liveNatsHealth     = liveNatsMonitor + "/healthz"
	liveNatsUser       = "mqstudio"
	liveNatsPassword   = "mqstudio"
	liveNatsSystemUser = "sys"
	liveNatsSystemPass = "sys"
)

func requireLiveNats(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.NATS,
		Name:   "the nats e2e cluster",
		Start:  "npm run e2e:nats:up",
		// /healthz rather than the client port: a JetStream server binds 4222
		// well before its meta group has elected a leader, and a connection
		// opened in that window finds a cluster that cannot answer anything
		// about its own streams.
		Probe: e2e.HTTPGet(liveNatsHealth),
	})
}

// natsStack is the connection service, the NATS service and the canonical
// services a board reads through, on a config directory of this test's own.
type natsStack struct {
	connections  *connection.Service
	nats         *natsservice.Service
	destinations *destination.Service
	messages     *message.Service
	consumers    *subscription.Service
}

func newNatsStack(t *testing.T) *natsStack {
	t.Helper()
	if _, ok := driver.Lookup(model.KindNATS); !ok {
		driver.Register(natsdriver.New())
	}

	paths := layout.In(t.TempDir())
	if err := crypto.InitKey(paths.Directory); err != nil {
		t.Fatalf("initialize encryption key: %v", err)
	}
	settingsService := settings.New(paths.SettingsFile)
	registry := driver.NewRegistry()
	t.Cleanup(registry.CloseAll)

	conns := newConnSource(registry)
	return &natsStack{
		connections: connection.New(
			paths.ConnectionsFile, settingsService, newRegistryRuntime(registry), newDescriptorEndpoints()),
		nats:         natsservice.New(conns, settingsService),
		destinations: destination.New(conns, settingsService),
		messages:     message.New(conns, settingsService),
		consumers:    subscription.New(conns, settingsService),
	}
}

// liveNatsProfile is the cluster as the APP account, with whichever of the two
// optional tiers the caller wants configured.
func liveNatsProfile(name string, withMonitor, withSystem bool) model.ConnectionProfile {
	profile := model.ConnectionProfile{
		Name:       name,
		Kind:       model.KindNATS,
		Endpoints:  liveNatsServers,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options:    map[string]string{},
		Secrets: map[string]string{
			natsdriver.SecretUsername: liveNatsUser,
			natsdriver.SecretPassword: liveNatsPassword,
		},
	}
	if withMonitor {
		profile.Options[natsdriver.OptionMonitorURL] = liveNatsMonitor
	}
	if withSystem {
		profile.Secrets[natsdriver.SecretSystemUser] = liveNatsSystemUser
		profile.Secrets[natsdriver.SecretSystemPassword] = liveNatsSystemPass
	}
	return profile
}

// dial stores a profile and opens it, returning the id a page would hold.
func (s *natsStack) dial(t *testing.T, profile model.ConnectionProfile) int {
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
 * The whole path in one go: store a profile, dial it, then declare a stream,
 * publish to it and read the message back through the id a page would pass.
 *
 * Writing and reading on the one connection is the point rather than a
 * shortcut. It is what the streams board, the send console and the message
 * browser are - three pages on one profile - and the write goes through the
 * NATS service while the reads go through the canonical ones, which is exactly
 * the split the boards use.
 */
func TestLiveNatsThroughAConnectionID(t *testing.T) {
	requireLiveNats(t)
	stack := newNatsStack(t)
	connID := stack.dial(t, liveNatsProfile("nats-live", true, true))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Named after this test so a parallel run against the same shared cluster
	// cannot collide, and torn down whatever happens below.
	stream := "MQS_APP_LIVE"
	subject := "mqs.app.live.orders"
	_ = stack.nats.DeleteStream(ctx, connID, stream)

	if err := stack.nats.SaveStream(ctx, connID, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: stream},
		Attributes: map[string]string{natsdriver.AttrSubjects: subject},
	}, false); err != nil {
		t.Fatalf("SaveStream: %v", err)
	}
	t.Cleanup(func() { _ = stack.nats.DeleteStream(context.Background(), connID, stream) })

	result, err := stack.nats.Publish(ctx, connID, natsdriver.PublishRequest{
		Subject: subject,
		Payload: "through-the-service",
		Persist: true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// Persisted rather than fire-and-forget, which is the difference this
	// assertion is for: core NATS acknowledges nothing, and a publish that
	// reported a sequence proves the stream took it.
	if result.Sequence == 0 {
		t.Errorf("the publish reported no sequence, so nothing was stored")
	}

	// The canonical listing, the way the streams board reads it.
	streams, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List destinations: %v", err)
	}
	var found *model.Destination
	for _, candidate := range streams {
		if candidate.Ref.Name == stream {
			found = candidate
		}
	}
	if found == nil {
		t.Fatalf("the stream this test created is not in the listing of %d", len(streams))
	}
	if found.Depth != 1 {
		t.Errorf("depth = %d, want the one message published", found.Depth)
	}

	// And the canonical browse, the way the messages board reads it.
	items, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{
		Topic:      stream,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("Query messages: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("browsed %d messages, want the one published", len(items))
	}
	if !strings.Contains(items[0].Body, "through-the-service") {
		t.Errorf("body = %q, want what was published", items[0].Body)
	}
	// The subject travels in Tags, which is the contract the messages board
	// reads: a NATS message has no key and no tag, and the subject is the one
	// thing that says where it came from.
	if !strings.Contains(items[0].Tags, subject) {
		t.Errorf("tags = %q, want the subject", items[0].Tags)
	}
}

/*
 * The credential round trip, which is the failure this project has already
 * had once.
 *
 * internal/service/connection used to assume a connection's only credentials
 * were RocketMQ's access key pair: saving dropped every other secret and
 * forced the mechanism to none, and the connection form's test button probed
 * the submitted profile rather than the stored one, so it passed on the way in
 * and the connection failed afterwards.
 *
 * On NATS a lost system credential is nearly invisible. The connection still
 * opens, every JetStream page still works, and only the cluster pages quietly
 * narrow from three servers to one - so this stores both pairs, reloads from
 * disk, dials what came back, and then proves the system pair is the one that
 * was used rather than that something connected.
 */
func TestLiveNatsCredentialsSurviveDisk(t *testing.T) {
	requireLiveNats(t)
	stack := newNatsStack(t)

	created, err := stack.connections.AddConnection(liveNatsProfile("nats-credentials", true, true))
	if err != nil {
		t.Fatalf("add connection: %v", err)
	}

	// Everything above could pass against a service that never wrote the
	// secrets. Reading them back off disk is the assertion.
	stored, err := stack.connections.GetConnection(created.ID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	for _, pair := range []struct{ key, want string }{
		{natsdriver.SecretUsername, liveNatsUser},
		{natsdriver.SecretPassword, liveNatsPassword},
		{natsdriver.SecretSystemUser, liveNatsSystemUser},
		{natsdriver.SecretSystemPassword, liveNatsSystemPass},
	} {
		if got := stored.Secret(pair.key); got != pair.want {
			t.Errorf("%s did not survive being stored: %q", pair.key, got)
		}
	}
	// The mechanism is what the old bug reset to none, and a profile that
	// kept its passwords and lost its mechanism authenticates as nobody.
	if stored.Auth.Mechanism != model.AuthPlain {
		t.Errorf("mechanism = %q, want plain", stored.Auth.Mechanism)
	}
	if stored.Option(natsdriver.OptionMonitorURL) != liveNatsMonitor {
		t.Errorf("the monitoring address did not survive being stored")
	}

	if err := stack.connections.Connect(created.ID); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = stack.connections.Disconnect(created.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The proof that the stored system credential is the one that was used.
	// Read through the monitoring endpoint every row would say "monitor" and
	// only one server's connections would appear; the fan-out is what makes
	// this different from a connection that merely opened.
	connections, err := stack.nats.Connections(ctx, created.ID, "")
	if err != nil {
		t.Fatalf("Connections through the stored credential: %v", err)
	}
	if len(connections) == 0 {
		t.Fatal("a cluster this app is connected to reported no connections at all")
	}
	servers := map[string]bool{}
	for _, connection := range connections {
		if connection.Attributes[natsdriver.AttrSource] != natsdriver.SourceSystem {
			t.Fatalf("connection %q was read via %q, so the system account was not used",
				connection.Name, connection.Attributes[natsdriver.AttrSource])
		}
		servers[connection.Node] = true
	}
	if len(servers) == 0 {
		t.Error("no connection named the server holding it")
	}
}

/*
 * The two mechanisms NATS added to the vocabulary, through disk.
 *
 * An nkey seed and a creds file path are new kinds of credential rather than
 * new names for a password, and neither is exercised by anything above: the
 * e2e cluster authenticates with a username. What can still be checked - and
 * is the half that broke before - is that storing one keeps both the secret
 * and the mechanism that says how to use it.
 */
func TestLiveNatsNewMechanismsSurviveDisk(t *testing.T) {
	requireLiveNats(t)
	stack := newNatsStack(t)

	for _, tc := range []struct {
		mechanism model.AuthMechanism
		key       string
		value     string
	}{
		{model.AuthNKey, natsdriver.SecretNKeySeed, "SUACSSL3UAHUDXKFSNVUZRF5UHPMWZ6BFDTJ7M6USDXIEDNPPQYYYCU3VY"},
		{model.AuthToken, natsdriver.SecretToken, "a-token-nobody-guesses"},
	} {
		t.Run(string(tc.mechanism), func(t *testing.T) {
			profile := liveNatsProfile("nats-"+string(tc.mechanism), false, false)
			profile.Auth = model.AuthConfig{Mechanism: tc.mechanism}
			profile.Secrets = map[string]string{tc.key: tc.value}

			created, err := stack.connections.AddConnection(profile)
			if err != nil {
				t.Fatalf("add connection: %v", err)
			}
			stored, err := stack.connections.GetConnection(created.ID)
			if err != nil {
				t.Fatalf("get connection: %v", err)
			}
			if got := stored.Secret(tc.key); got != tc.value {
				t.Errorf("%s did not survive being stored: %q", tc.key, got)
			}
			if stored.Auth.Mechanism != tc.mechanism {
				t.Errorf("mechanism = %q, want %q", stored.Auth.Mechanism, tc.mechanism)
			}
		})
	}
}

/*
 * A capability the connection does not have is refused by name.
 *
 * The service checks the declared capability before the type assertion, so an
 * operation a tier cannot answer comes back saying which capability is missing
 * rather than which Go interface was not implemented. That is what puts a
 * readable sentence on the board instead of a stack trace.
 *
 * Closing a connection is the sharpest case: the driver implements the port
 * unconditionally and only the credentials decide, so without the check this
 * would reach the driver and fail somewhere the page cannot explain.
 */
func TestLiveNatsWithoutTheSystemAccountRefusesByCapability(t *testing.T) {
	requireLiveNats(t)
	stack := newNatsStack(t)
	connID := stack.dial(t, liveNatsProfile("nats-no-system", true, false))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := stack.nats.CloseConnection(ctx, connID, "nats-1/1", "no system account")
	if err == nil {
		t.Fatal("closing a connection was accepted without the system account")
	}
	var unsupported *driver.UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Fatalf("CloseConnection said %v, want an unsupported capability", err)
	}
	if unsupported.Capability != model.CapClientClose {
		t.Errorf("refused %s, want %s", unsupported.Capability, model.CapClientClose)
	}

	// And the half that still works, through the monitoring endpoint alone.
	// Listing is not a request the endpoint refuses, so the page keeps it.
	if _, err := stack.nats.Connections(ctx, connID, ""); err != nil {
		t.Errorf("Connections without the system account: %v", err)
	}
}

/*
 * A consumer, its backlog and its lag, through the canonical subscription
 * service.
 *
 * The consumers board reads this listing rather than anything NATS-specific,
 * so a driver that reported a consumer correctly to its own tests and mapped
 * it wrong onto model.Subscription would show an empty page and nothing here
 * would say why.
 */
func TestLiveNatsConsumersThroughTheCanonicalService(t *testing.T) {
	requireLiveNats(t)
	stack := newNatsStack(t)
	connID := stack.dial(t, liveNatsProfile("nats-consumers", true, true))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream := "MQS_APP_CONSUMERS"
	subject := "mqs.app.consumers.>"
	_ = stack.nats.DeleteStream(ctx, connID, stream)
	if err := stack.nats.SaveStream(ctx, connID, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: stream},
		Attributes: map[string]string{natsdriver.AttrSubjects: subject},
	}, false); err != nil {
		t.Fatalf("SaveStream: %v", err)
	}
	t.Cleanup(func() { _ = stack.nats.DeleteStream(context.Background(), connID, stream) })

	if err := stack.nats.SaveConsumer(ctx, connID, model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: stream, Name: "app-worker"},
	}, false); err != nil {
		t.Fatalf("SaveConsumer: %v", err)
	}

	// Three messages, so the backlog below is a number rather than a zero
	// that would pass whatever the mapping did.
	for i := range 3 {
		if _, err := stack.nats.Publish(ctx, connID, natsdriver.PublishRequest{
			Subject: fmt.Sprintf("mqs.app.consumers.%d", i),
			Payload: "queued",
			Persist: true,
		}); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	consumers, err := stack.consumers.List(ctx, connID)
	if err != nil {
		t.Fatalf("List consumers: %v", err)
	}
	var worker *model.Subscription
	for _, candidate := range consumers {
		if candidate.Ref.Namespace == stream && candidate.Ref.Name == "app-worker" {
			worker = candidate
		}
	}
	if worker == nil {
		t.Fatalf("the consumer this test created is not in the listing of %d", len(consumers))
	}
	if worker.Backlog != 3 {
		t.Errorf("backlog = %d, want the three messages published", worker.Backlog)
	}
	// A pull consumer holds nothing open between fetches, so there is nobody
	// to count - and reporting zero would say a working consumer is idle.
	if worker.Members != model.UnknownMetric {
		t.Errorf("members = %d, want UnknownMetric for a pull consumer", worker.Members)
	}
}
