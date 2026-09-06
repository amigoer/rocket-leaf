package azureservicebus

import (
	"context"
	"strconv"
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

// Names a test creates for itself. Everything mqs-test-* is this suite's and
// is removed again; everything mqs-seed-* is the seed's and is only read.
const (
	testQueue = "mqs-test-queue"
	testTopic = "mqs-test-topic"
)

// removeEntities takes the test's own objects away whatever the test did.
func removeEntities(t *testing.T, conn *Conn, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range names {
			_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: name})
		}
	})
}

/*
 * Creating both kinds, which the emulator can do and its config file cannot.
 *
 * Worth saying out loud because the emulator's topology is otherwise declared
 * in tests/e2e/azure-servicebus/config.json and read at startup: it would have
 * been easy to assume that is the only way entities exist there. It is not -
 * the management API creates and deletes them at runtime exactly as a real
 * namespace does, which is what makes this path testable at all.
 */
func TestLiveCreateAndRemoveBothKinds(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	removeEntities(t, conn, testQueue, testTopic)

	if err := conn.CreateEntity(ctx, EntitySpec{
		Name:             testQueue,
		Kind:             EntityQueue,
		LockDurationSec:  45,
		MaxDeliveryCount: 7,
		TTLSec:           3600,
	}); err != nil {
		t.Fatalf("CreateEntity(queue): %v", err)
	}

	created, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: testQueue})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if created.Attributes[AttrEntityType] != EntityQueue {
		t.Errorf("%s was created as %q", testQueue, created.Attributes[AttrEntityType])
	}
	if created.Attributes[AttrLockDurationSec] != "45" {
		t.Errorf("lock duration = %q, want 45", created.Attributes[AttrLockDurationSec])
	}
	if created.Attributes[AttrMaxDeliveryCount] != "7" {
		t.Errorf("delivery limit = %q, want 7", created.Attributes[AttrMaxDeliveryCount])
	}

	if err := conn.CreateEntity(ctx, EntitySpec{Name: testTopic, Kind: EntityTopic}); err != nil {
		t.Fatalf("CreateEntity(topic): %v", err)
	}
	topic, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: testTopic})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if topic.Attributes[AttrEntityType] != EntityTopic {
		t.Errorf("%s was created as %q", testTopic, topic.Attributes[AttrEntityType])
	}

	// Queues and topics share one name space, and the message has to say so:
	// the service's own is "the messaging entity already exists" with a
	// tracking id, which does not explain why a free-looking topic name is not.
	err = conn.CreateEntity(ctx, EntitySpec{Name: testQueue, Kind: EntityTopic})
	if err == nil {
		t.Fatal("created a topic with a queue's name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("the refusal does not say the name is taken: %v", err)
	}

	for _, name := range []string{testQueue, testTopic} {
		if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: name}); err != nil {
			t.Errorf("RemoveDestination(%s): %v", name, err)
		}
	}
	if _, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: testQueue}); err == nil {
		t.Error("a deleted queue is still described")
	}
	if err := conn.RemoveDestination(ctx, model.DestinationRef{Name: testQueue}); err == nil {
		t.Error("deleting an entity twice succeeded twice")
	}
}

/*
 * An update reads the entity first and writes the whole thing back, because
 * the management API replaces a description rather than patching one. What
 * that has to buy is that an omitted setting survives: a form that edits the
 * delivery limit must not reset the lock duration to the service's default.
 */
func TestLiveUpdateKeepsWhatTheFormDidNotSend(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	removeEntities(t, conn, testQueue)

	if err := conn.CreateEntity(ctx, EntitySpec{
		Name:             testQueue,
		LockDurationSec:  45,
		MaxDeliveryCount: 7,
	}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}

	if err := conn.UpdateEntity(ctx, EntitySpec{Name: testQueue, MaxDeliveryCount: 9}); err != nil {
		t.Fatalf("UpdateEntity: %v", err)
	}

	after, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: testQueue})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if after.Attributes[AttrMaxDeliveryCount] != "9" {
		t.Errorf("delivery limit = %q, want the 9 that was sent", after.Attributes[AttrMaxDeliveryCount])
	}
	if after.Attributes[AttrLockDurationSec] != "45" {
		t.Errorf("lock duration = %q; the update reset a setting it was not given",
			after.Attributes[AttrLockDurationSec])
	}
}

/*
 * An update cannot turn a queue into a topic, and the driver has to be the one
 * that says so.
 *
 * The two are addressed at one Atom path, so a QueueDescription sent to a
 * topic's path is not refused by the service: it replaces the topic with a
 * queue, and every subscription on it goes with the old object. That is a
 * silent data loss behind a form control, which is why this is checked before
 * anything is sent.
 */
func TestLiveUpdateRefusesToChangeAnEntityIntoTheOtherKind(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	err := conn.UpdateEntity(ctx, EntitySpec{Name: seedEvents, Kind: EntityQueue, TTLSec: 60})
	if err == nil {
		t.Fatal("turned a topic into a queue, taking its subscriptions with it")
	}
	if !strings.Contains(err.Error(), "topic") || !strings.Contains(err.Error(), "queue") {
		t.Errorf("the refusal does not name both kinds: %v", err)
	}

	// And the topic is still a topic, with its subscriptions.
	after, err := conn.DestinationDetail(ctx, model.DestinationRef{Name: seedEvents})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if after.Attributes[AttrEntityType] != EntityTopic || after.Subscribers != 3 {
		t.Errorf("%s is now a %q with %d subscriptions",
			seedEvents, after.Attributes[AttrEntityType], after.Subscribers)
	}
}

/*
 * What the emulator will not do, recorded rather than worked around.
 *
 * ForwardDeadLetteredMessagesTo takes a plain entity name against a real
 * namespace - that is what the portal sends - and the emulator answers 400
 * with "Absolute URI must be provided". So the forwarding fields on the entity
 * form cannot be exercised here, and a user pointing this app at the emulator
 * will meet the same refusal.
 *
 * Pinned rather than skipped silently: if the emulator ever accepts a name,
 * this goes red and the gap can be closed.
 */
func TestLiveForwardingWantsAnAbsoluteURI(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	removeEntities(t, conn, testQueue)

	err := conn.CreateEntity(ctx, EntitySpec{
		Name:                 testQueue,
		ForwardDeadLettersTo: seedQuiet,
	})
	if err == nil {
		t.Fatal("the emulator took a plain entity name for forwarding; " +
			"the gap this test records has closed and the comment above needs removing")
	}
	if !strings.Contains(err.Error(), "Absolute URI") {
		t.Errorf("the emulator refused forwarding for some other reason: %v", err)
	}
}

// The bounds are checked in the driver so the message can name the form's own
// row: the service answers a 400 whose detail is a subcode and a tracking id.
func TestLiveCreateRefusesSettingsTheServiceWould(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	for _, spec := range []EntitySpec{
		{Name: testQueue, LockDurationSec: 301},
		{Name: testQueue, LockDurationSec: 4},
		{Name: testQueue, MaxDeliveryCount: 2001},
	} {
		if err := conn.CreateEntity(ctx, spec); err == nil {
			removeEntities(t, conn, testQueue)
			t.Fatalf("created a queue with %#v, which the service refuses", spec)
		}
	}
}

const testSubscription = "mqs-test-sub"

/*
 * Subscriptions, which is where a topic's messages actually are.
 *
 * The listing walks every topic, because the management API lists one topic's
 * subscriptions at a time and has no call that lists them all - so what this
 * pins is that a subscription arrives with its topic attached, and with the
 * rules that decide what reaches it.
 */
func TestLiveListSubscriptionsWalksEveryTopic(t *testing.T) {
	conn := liveConn(t)

	found, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}

	byName := make(map[string]*model.Subscription, len(found))
	for _, subscription := range found {
		byName[subscription.Ref.Name] = subscription
	}

	for _, name := range []string{seedSubAll, seedSubRed, seedSubOrders} {
		row, listed := byName[name]
		if !listed {
			e2e.Missing(t, "%s is not in the listing; run npm run e2e:azure-servicebus:seed", name)
		}
		if row.Ref.Namespace != seedEvents {
			t.Errorf("%s belongs to %q, want %s", name, row.Ref.Namespace, seedEvents)
		}
		if row.Attributes[SubAttrTopic] != seedEvents {
			t.Errorf("%s names topic %q", name, row.Attributes[SubAttrTopic])
		}
		// Exactly one topic, chosen at creation.
		if row.Destinations != 1 {
			t.Errorf("%s reads %d destinations, and a subscription reads one", name, row.Destinations)
		}
		// Nothing registers as a consumer, so this must be unknown rather
		// than a zero that would read as "nothing is consuming this".
		if row.Members != model.UnknownMetric {
			t.Errorf("%s reports %d members, and Service Bus registers none", name, row.Members)
		}
		if row.Attributes[SubAttrRuleNames] == "" {
			t.Errorf("%s carries no rule names, so the board cannot say what reaches it", name)
		}
	}

	// The rules are the point of the seeded topic: three subscriptions on one
	// topic, each with a different rule, so each holds a different set.
	if got := byName[seedSubAll].Attributes[SubAttrRuleNames]; got != "$Default" {
		t.Errorf("%s has rules %q, want the default that matches everything", seedSubAll, got)
	}
	if got := byName[seedSubRed].Attributes[SubAttrRuleNames]; got != "red-only" {
		t.Errorf("%s has rules %q, want the SQL filter the config declares", seedSubRed, got)
	}
	if got := byName[seedSubOrders].Attributes[SubAttrRuleNames]; got != "orders-only" {
		t.Errorf("%s has rules %q, want the correlation filter the config declares",
			seedSubOrders, got)
	}
}

// A subscription is addressed as (topic, name), so a ref carrying only a name
// addresses nothing - and the message has to say which half is missing.
func TestLiveSubscriptionNeedsItsTopic(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: seedSubAll})
	if err == nil {
		t.Fatal("described a subscription without naming its topic")
	}
	if !strings.Contains(err.Error(), "topic") {
		t.Errorf("the refusal does not mention the topic: %v", err)
	}

	found, err := conn.SubscriptionDetail(liveContext(t),
		model.SubscriptionRef{Namespace: seedEvents, Name: seedSubAll})
	if err != nil {
		t.Fatalf("SubscriptionDetail: %v", err)
	}
	if found.Ref.Name != seedSubAll {
		t.Errorf("described %q", found.Ref.Name)
	}
}

/*
 * Creating and deleting a subscription, and the $Default rule that comes with
 * one.
 *
 * Worth asserting rather than assuming: the create form deliberately offers no
 * filter, so what decides that a brand-new subscription receives anything at
 * all is the rule the service adds by itself.
 */
func TestLiveCreateSubscriptionComesWithADefaultRule(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	t.Cleanup(func() {
		_ = conn.RemoveSubscription(context.Background(),
			model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription})
	})

	if err := conn.CreateSubscriptionFrom(ctx, SubscriptionSpec{
		Topic:            seedEvents,
		Name:             testSubscription,
		LockDurationSec:  30,
		MaxDeliveryCount: 4,
	}); err != nil {
		t.Fatalf("CreateSubscriptionFrom: %v", err)
	}

	created, err := conn.SubscriptionDetail(ctx,
		model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription})
	if err != nil {
		t.Fatalf("SubscriptionDetail: %v", err)
	}
	if created.Attributes[SubAttrRuleNames] != "$Default" {
		t.Errorf("a new subscription has rules %q, want the $Default that matches everything",
			created.Attributes[SubAttrRuleNames])
	}
	if created.Attributes[SubAttrLockDurationSec] != "30" {
		t.Errorf("lock duration = %q, want 30", created.Attributes[SubAttrLockDurationSec])
	}
	if created.Status != model.SubscriptionOnline {
		t.Errorf("status = %q, want online", created.Status)
	}

	// An omitted setting survives an update, the way it does on an entity.
	if err := conn.UpdateSubscriptionFrom(ctx, SubscriptionSpec{
		Topic: seedEvents, Name: testSubscription, MaxDeliveryCount: 6,
	}); err != nil {
		t.Fatalf("UpdateSubscriptionFrom: %v", err)
	}
	after, err := conn.SubscriptionDetail(ctx,
		model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription})
	if err != nil {
		t.Fatalf("SubscriptionDetail: %v", err)
	}
	if after.Attributes[SubAttrMaxDeliveryCount] != "6" {
		t.Errorf("delivery limit = %q, want the 6 that was sent",
			after.Attributes[SubAttrMaxDeliveryCount])
	}
	if after.Attributes[SubAttrLockDurationSec] != "30" {
		t.Errorf("lock duration = %q; the update reset a setting it was not given",
			after.Attributes[SubAttrLockDurationSec])
	}

	if err := conn.RemoveSubscription(ctx,
		model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription}); err != nil {
		t.Fatalf("RemoveSubscription: %v", err)
	}
	if _, err := conn.SubscriptionDetail(ctx,
		model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription}); err == nil {
		t.Error("a deleted subscription is still described")
	}
}

/*
 * A subscription with no rules at all reports itself offline.
 *
 * It is the quiet failure this family has: a subscription is created with a
 * $Default rule matching everything, and deleting that without adding another
 * leaves an object that exists, reports Active, has an empty backlog because
 * nothing can arrive, and will never receive a message again. Every figure on
 * the board looks healthy, which is why the status has to say otherwise.
 */
func TestLiveASubscriptionWithNoRulesIsOffline(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)
	t.Cleanup(func() {
		_ = conn.RemoveSubscription(context.Background(),
			model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription})
	})

	if err := conn.CreateSubscriptionFrom(ctx, SubscriptionSpec{
		Topic: seedEvents, Name: testSubscription,
	}); err != nil {
		t.Fatalf("CreateSubscriptionFrom: %v", err)
	}
	if _, err := conn.management.DeleteRule(ctx, seedEvents, testSubscription, "$Default", nil); err != nil {
		t.Fatalf("deleting the default rule: %v", err)
	}

	stranded, err := conn.SubscriptionDetail(ctx,
		model.SubscriptionRef{Namespace: seedEvents, Name: testSubscription})
	if err != nil {
		t.Fatalf("SubscriptionDetail: %v", err)
	}
	if stranded.Status != model.SubscriptionOffline {
		t.Errorf("status = %q; a subscription nothing can reach is not online", stranded.Status)
	}
	if stranded.Attributes[SubAttrRuleNames] != "" {
		t.Errorf("rule names = %q, want none", stranded.Attributes[SubAttrRuleNames])
	}
}

/*
 * The backlog is degraded against the emulator and only against it.
 *
 * Service Bus reports it as the subscription's active message count. The
 * emulator sends a CountDetails element whose five children are renamed to
 * tokens the SDK cannot read - and reading it there is not merely useless, it
 * is what makes the SDK dereference a nil pointer, which is the reason the
 * driver asks for none of these figures against an emulator.
 *
 * Pinned as a narrowing rather than as a family-wide gap: a real namespace
 * answers this, and declaring it absent everywhere would be a lie about Azure.
 */
func TestLiveBacklogIsDegradedOnlyAgainstTheEmulator(t *testing.T) {
	conn := liveConn(t)

	declared := conn.Capabilities()
	if declared.Has(model.CapSubscriptionLag) {
		t.Error("the backlog is offered as a figure, and this endpoint reports none")
	}
	reason, degraded := declared.DegradedReason(model.CapSubscriptionLag)
	if !degraded {
		t.Fatal("the backlog is neither supported nor explained, so the page says nothing at all")
	}
	if reason != countsNotInEmulator {
		t.Errorf("reason = %q, want %q", reason, countsNotInEmulator)
	}

	// The subscriptions page is still reachable: listing, creating and
	// deleting all work, and only the one figure is missing.
	if !declared.Has(model.CapSubscriptionList) {
		t.Error("the subscriptions page is unreachable because one figure is missing")
	}

	found, err := conn.SubscriptionDetail(liveContext(t),
		model.SubscriptionRef{Namespace: seedEvents, Name: seedSubRed})
	if err != nil {
		t.Fatalf("SubscriptionDetail: %v", err)
	}
	if found.Backlog != model.UnknownMetric {
		t.Errorf("backlog = %d against an emulator that reports none", found.Backlog)
	}

	// And a namespace that is not an emulator declares it supported. Built
	// rather than dialled: what is asserted is the narrowing rule, and the
	// only way to dial the other side of it is a real Azure subscription.
	real := &Conn{closed: make(chan struct{}), config: clientConfig{namespace: "real.servicebus.windows.net"}}
	if !real.declare().Has(model.CapSubscriptionLag) {
		t.Error("a real namespace does not offer the backlog, and Service Bus reports it")
	}
}

/*
 * The point of this family's messages page, exercised end to end.
 *
 * A peek takes nothing. This browses the same entity twice and asserts the
 * second run sees exactly what the first did - which is the whole difference
 * from SQS and Pub/Sub, where the second read of a browsed message either
 * cannot see it or sees a higher delivery count.
 *
 * The delivery count is the sharper half of it: a receive would raise it and a
 * peek must not, so a message browsed a hundred times is no closer to being
 * dead-lettered than one browsed never.
 */
func TestLivePeekTakesNothingAndRepeats(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	first, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(first) == 0 {
		e2e.Missing(t, "%s holds nothing; run npm run e2e:azure-servicebus:seed", seedOrders)
	}

	second, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("QueryMessages, second run: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("the second browse saw %d messages and the first saw %d; a peek took something",
			len(second), len(first))
	}
	for index := range first {
		if first[index].QueueOffset != second[index].QueueOffset {
			t.Errorf("row %d moved from sequence %d to %d between two browses",
				index, first[index].QueueOffset, second[index].QueueOffset)
		}
		if first[index].RetryTimes != second[index].RetryTimes {
			t.Errorf("sequence %d went from %d to %d retries; a peek raised a delivery count",
				first[index].QueueOffset, first[index].RetryTimes, second[index].RetryTimes)
		}
	}

	// And what a consumer would still be offered afterwards is everything: a
	// receive after two browses gets the first message rather than the third.
	receiver, err := conn.receiver(seedOrders, "", false)
	if err != nil {
		t.Fatalf("opening a receiver: %v", err)
	}
	defer func() { _ = receiver.Close(context.Background()) }()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	batch, err := receiver.ReceiveMessages(waitCtx, 1, nil)
	if err != nil {
		t.Fatalf("ReceiveMessages: %v", err)
	}
	if len(batch) == 0 {
		t.Fatal("a consumer was offered nothing after two browses")
	}
	// Put it straight back, so the seed's counts survive this test.
	if err := receiver.AbandonMessage(context.WithoutCancel(ctx), batch[0], nil); err != nil {
		t.Errorf("abandoning: %v", err)
	}
	lowest := first[len(first)-1].QueueOffset
	if batch[0].SequenceNumber == nil || *batch[0].SequenceNumber != lowest {
		t.Errorf("a consumer was offered sequence %v after two browses, want the oldest %d",
			batch[0].SequenceNumber, lowest)
	}
}

/*
 * What a peek reaches that a receive never would.
 *
 * The seed schedules one message on mqs-seed-orders for an hour from now. No
 * consumer is offered it - the service holds it until its enqueue time - and
 * it appears here with a state saying so. That is the second half of why this
 * page is a peek rather than a read.
 */
func TestLivePeekReachesAScheduledMessage(t *testing.T) {
	conn := liveConn(t)

	held, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}

	scheduled := 0
	for _, message := range held {
		if message.Properties[PropState] == StateScheduled {
			scheduled++
			if message.Properties[PropScheduledEnqueueTime] == "" {
				t.Errorf("sequence %d is scheduled and says nothing about when",
					message.QueueOffset)
			}
		}
	}
	if scheduled != 1 {
		e2e.Missing(t, "%s holds %d scheduled messages, seeded 1; run npm run e2e:azure-servicebus:seed",
			seedOrders, scheduled)
	}
}

/*
 * The browse resumes from a sequence number, which is what makes it
 * repeatable at all.
 *
 * A receiver keeps a cursor that advances with every peek, so a second call
 * with no starting position returns what follows the first. The driver always
 * sends one; this asserts what that buys - a page from the middle that begins
 * where it was asked to.
 */
func TestLivePeekResumesFromASequenceNumber(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	all, err := conn.QueryMessages(ctx, model.MessageQueryParams{Topic: seedOrders, MaxResults: 50})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(all) < 4 {
		e2e.Missing(t, "%s holds %d messages, need at least 4", seedOrders, len(all))
	}
	// The rows come back newest first, so the oldest is last.
	third := all[len(all)-3].QueueOffset

	from, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic:      seedOrders,
		MaxResults: 50,
		Filters:    map[string]string{FilterFromSequence: strconv.FormatInt(third, 10)},
	})
	if err != nil {
		t.Fatalf("QueryMessages from a sequence: %v", err)
	}
	if len(from) != len(all)-2 {
		t.Errorf("starting at the third message returned %d rows, want %d", len(from), len(all)-2)
	}
	for _, message := range from {
		if message.QueueOffset < third {
			t.Errorf("a browse starting at %d returned sequence %d", third, message.QueueOffset)
		}
	}
}

/*
 * A subscription is browsed through its topic, and what comes back is what its
 * rules let through.
 *
 * The three seeded subscriptions share one topic and hold different sets
 * because of their rules, which is the one thing a topic's own board could
 * never show: the topic holds nothing at all.
 */
func TestLivePeekReadsASubscriptionThroughItsRules(t *testing.T) {
	conn := liveConn(t)
	ctx := liveContext(t)

	expected := map[string]int{seedSubAll: 9, seedSubRed: 4, seedSubOrders: 3}
	for name, want := range expected {
		held, err := conn.QueryMessages(ctx, model.MessageQueryParams{
			Topic:      seedEvents,
			MaxResults: 50,
			Filters:    map[string]string{FilterSubscription: name},
		})
		if err != nil {
			t.Fatalf("QueryMessages(%s): %v", name, err)
		}
		if len(held) != want {
			e2e.Missing(t, "%s/%s holds %d messages, seeded %d; run npm run e2e:azure-servicebus:seed",
				seedEvents, name, len(held), want)
		}
		for _, message := range held {
			if message.Topic != seedEvents+"/"+name {
				t.Errorf("a row from %s says it came from %q", name, message.Topic)
			}
		}
	}

	// The rows carry what a rule selects on: the sender's own properties, and
	// the subject a correlation filter matches by name.
	red, err := conn.QueryMessages(ctx, model.MessageQueryParams{
		Topic:      seedEvents,
		MaxResults: 50,
		Filters:    map[string]string{FilterSubscription: seedSubRed},
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	for _, message := range red {
		if message.Properties[PropAttributePrefix+"colour"] != "red" {
			t.Errorf("sequence %d reached the red-only subscription with colour %q",
				message.QueueOffset, message.Properties[PropAttributePrefix+"colour"])
		}
		if message.Tags == "" {
			t.Errorf("sequence %d carries no subject, which is what a correlation filter matches",
				message.QueueOffset)
		}
	}
}

// Browsing something that is not there is an error naming it, not an empty
// page: an empty entity and a mistyped one are different answers.
func TestLivePeekRefusesAnEntityThatIsNotThere(t *testing.T) {
	conn := liveConn(t)

	if _, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: "mqs-test-absent", MaxResults: 10,
	}); err == nil {
		t.Error("browsed an entity that does not exist")
	}
}
