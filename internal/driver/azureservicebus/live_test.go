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

/*
 * Listing, which is two API calls folded into one board.
 *
 * Queues and topics are separate resources in the management API and one board
 * here, so what this pins is that both arrive, that each row says which it is,
 * and that a topic's subscription count comes back - it is a further call per
 * topic and the figure the row exists for.
 */
func TestLiveListDestinationsCoversQueuesAndTopics(t *testing.T) {
	conn := liveConn(t)

	found, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}

	byName := make(map[string]*model.Destination, len(found))
	for _, destination := range found {
		byName[destination.Ref.Name] = destination
	}

	for _, name := range []string{seedOrders, seedFailures, seedQuiet} {
		row, listed := byName[name]
		if !listed {
			e2e.Missing(t, "%s is not in the listing; run npm run e2e:azure-servicebus:seed", name)
		}
		if got := row.Attributes[AttrEntityType]; got != EntityQueue {
			t.Errorf("%s is listed as %q, want a queue", name, got)
		}
		// A queue has no subscribers and the service keeps no register of who
		// is reading it, so this must be unknown rather than zero.
		if row.Subscribers != model.UnknownMetric {
			t.Errorf("%s reports %d subscribers, and Service Bus registers none", name, row.Subscribers)
		}
	}

	events, listed := byName[seedEvents]
	if !listed {
		e2e.Missing(t, "%s is not in the listing; run npm run e2e:azure-servicebus:seed", seedEvents)
	}
	if got := events.Attributes[AttrEntityType]; got != EntityTopic {
		t.Errorf("%s is listed as %q, want a topic", seedEvents, got)
	}
	if events.Subscribers != 3 {
		t.Errorf("%s reports %d subscriptions, seeded 3", seedEvents, events.Subscribers)
	}
	for _, name := range []string{seedSubAll, seedSubRed, seedSubOrders} {
		if !strings.Contains(events.Attributes[AttrSubscriptionNames], name) {
			t.Errorf("%s does not name %s among its subscriptions: %q",
				seedEvents, name, events.Attributes[AttrSubscriptionNames])
		}
	}

	// The topic the seed creates through the management API, because the
	// emulator's own config file refuses to declare one with no subscriptions.
	orphaned, listed := byName[seedOrphaned]
	if !listed {
		e2e.Missing(t, "%s is not in the listing; run npm run e2e:azure-servicebus:seed", seedOrphaned)
	}
	if orphaned.Subscribers != 0 {
		t.Errorf("%s reports %d subscriptions; it exists to have none", seedOrphaned, orphaned.Subscribers)
	}
}

/*
 * A topic's depth is never a number, and that is the family rather than the
 * emulator.
 *
 * A topic holds nothing: a send is copied into every subscription whose rules
 * let it through and discarded if none do. A zero here would say "this topic
 * is empty", which is true of every topic that ever existed and tells a reader
 * nothing about whether their messages went anywhere.
 */
func TestLiveATopicReportsNoDepth(t *testing.T) {
	conn := liveConn(t)

	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedEvents})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if topic.Depth != model.UnknownMetric {
		t.Errorf("%s reports a depth of %d, and a topic holds no messages", seedEvents, topic.Depth)
	}
	if topic.Partitions != model.UnknownMetric {
		t.Errorf("%s reports %d partitions, and partitioning is a flag rather than a count",
			seedEvents, topic.Partitions)
	}
}

/*
 * What this environment cannot exercise, asserted rather than stepped around.
 *
 * Service Bus reports a queue's depth and a subscription's backlog in the
 * CountDetails element of the entity's Atom description. The emulator serves
 * that element for a queue and a topic not at all, and for a subscription with
 * its five children renamed to obfuscated tokens - so the SDK returns an error
 * on the first and dereferences a nil pointer on the second.
 *
 * The driver's answer is to ask for none of them against an emulator and
 * report an unknown depth, and to say so through a degraded capability with a
 * reason rather than by printing a zero. This pins that: a number appearing
 * here would mean either that the emulator started reporting counts or that
 * the driver started inventing them, and those need telling apart.
 */
func TestLiveCountsAreDegradedAgainstTheEmulator(t *testing.T) {
	conn := liveConn(t)

	queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedOrders})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if queue.Depth != model.UnknownMetric {
		t.Errorf("%s reports a depth of %d against an emulator that reports none", seedOrders, queue.Depth)
	}
	if _, given := queue.Attributes[AttrDeadLetterCount]; given {
		t.Errorf("%s reports a dead-letter count against an emulator that reports none", seedOrders)
	}

	// And the guard itself: the call the SDK panics on is never made here, but
	// it must be survivable if it ever is.
	if _, known := guardedCounts(func() (counts, error) { panic("obfuscated CountDetails") }); known {
		t.Error("a panicking count read was reported as a known figure")
	}
}

// Detail takes a name and works out which of the two it is, because a delete
// or an inspect confirmed by name should not depend on the page having
// remembered.
func TestLiveDestinationDetailFindsEitherKindByName(t *testing.T) {
	conn := liveConn(t)

	queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedOrders})
	if err != nil {
		t.Fatalf("DestinationDetail(queue): %v", err)
	}
	if queue.Attributes[AttrEntityType] != EntityQueue {
		t.Errorf("%s came back as %q", seedOrders, queue.Attributes[AttrEntityType])
	}

	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedEvents})
	if err != nil {
		t.Fatalf("DestinationDetail(topic): %v", err)
	}
	if topic.Attributes[AttrEntityType] != EntityTopic {
		t.Errorf("%s came back as %q", seedEvents, topic.Attributes[AttrEntityType])
	}

	if _, err := conn.DestinationDetail(liveContext(t),
		model.DestinationRef{Name: "mqs-test-not-here"}); err == nil {
		t.Error("described an entity that does not exist")
	}
}

// The prefix is the connection's own filter and the API has none, so it is
// applied in the driver - which means it has to be applied to both listings.
func TestLiveEntityPrefixNarrowsBothListings(t *testing.T) {
	requireNamespace(t)

	profile := liveProfile()
	profile.Options[OptionEntityPrefix] = "mqs-seed-e"
	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	found, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	if len(found) != 1 || found[0].Ref.Name != seedEvents {
		names := make([]string, 0, len(found))
		for _, destination := range found {
			names = append(names, destination.Ref.Name)
		}
		t.Fatalf("the prefix let through %v, want only %s", names, seedEvents)
	}
	// The prefix filters the boards; how many readers a topic has is a fact
	// about the topic, so the count must not be narrowed with it.
	if found[0].Subscribers != 3 {
		t.Errorf("%s reports %d subscriptions under a prefix, want the true 3",
			seedEvents, found[0].Subscribers)
	}
}

/*
 * The API's sharpest edge, pinned.
 *
 * A queue and a topic are addressed at the same Atom path, and the SDK's
 * GetQueue and GetTopic send exactly the same request - they differ only in
 * which element they look for in the reply. So GetQueue on a topic does not
 * fail and does not answer nil: it parses a TopicDescription looking for a
 * QueueDescription, finds none, and returns a response with every field nil.
 *
 * A driver that trusted a non-nil response would report every topic as a queue
 * with no settings at all, and nothing else would go red. Worse, the same
 * sharing applies to DELETE: DeleteQueue on a topic's name removes the topic
 * and every subscription on it.
 *
 * This asserts the shape of the trap rather than only the driver's answer to
 * it, so a future SDK that starts returning nil - or an error - is a red test
 * rather than a silent change of behaviour underneath describeEntity.
 */
func TestLiveGetQueueDoesNotRefuseATopicName(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	crossed, err := conn.management.GetQueue(ctx, seedEvents, nil)
	if err != nil {
		t.Fatalf("GetQueue on a topic: %v", err)
	}
	if crossed == nil {
		t.Skip("[e2e-gate] the SDK now answers nil for a cross-kind get; describeEntity can be simplified")
	}
	if crossed.Status != nil {
		t.Errorf("GetQueue on a topic came back with a status of %q, so the discriminator is gone",
			*crossed.Status)
	}

	// And the driver's answer to it.
	described, err := conn.describeEntity(ctx, seedEvents)
	if err != nil {
		t.Fatalf("describeEntity: %v", err)
	}
	if described.kind() != EntityTopic {
		t.Errorf("%s is described as %q", seedEvents, described.kind())
	}
	described, err = conn.describeEntity(ctx, seedOrders)
	if err != nil {
		t.Fatalf("describeEntity: %v", err)
	}
	if described.kind() != EntityQueue {
		t.Errorf("%s is described as %q", seedOrders, described.kind())
	}
	described, err = conn.describeEntity(ctx, "mqs-test-absent")
	if err != nil {
		t.Fatalf("describeEntity: %v", err)
	}
	if described.kind() != "" {
		t.Errorf("an entity that does not exist is described as %q", described.kind())
	}
}
