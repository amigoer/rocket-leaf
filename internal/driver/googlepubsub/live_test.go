package googlepubsub

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	pubsub "cloud.google.com/go/pubsub/v2"

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

func findDestination(list []*model.Destination, name string) *model.Destination {
	for _, entry := range list {
		if entry.Ref.Name == name {
			return entry
		}
	}
	return nil
}

/*
 * The listing, and the one figure it exists to show.
 *
 * A Pub/Sub topic holds nothing, so there is no depth to compare and the
 * subscription count is the whole of what separates a working topic from one
 * that is discarding everything published to it.
 */
func TestLiveListDestinationsCountsWhatReadsEachTopic(t *testing.T) {
	conn := liveConn(t)

	topics, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	orders := findDestination(topics, seedOrders)
	if orders == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", seedOrders)
	}
	if orders.Subscribers != 2 {
		t.Errorf("%s reports %d subscriptions, want the 2 the seed creates",
			seedOrders, orders.Subscribers)
	}
	names := orders.Attribute(AttrSubscriptionNames)
	if !strings.Contains(names, seedWorker) || !strings.Contains(names, seedAudit) {
		t.Errorf("%s names its subscriptions as %q, want both seeded ones", seedOrders, names)
	}

	orphaned := findDestination(topics, seedOrphaned)
	if orphaned == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", seedOrphaned)
	}
	// The state the alerts page fires on: nothing is subscribed, so every
	// publish is discarded on the spot.
	if orphaned.Subscribers != 0 {
		t.Errorf("%s reports %d subscriptions, want none", seedOrphaned, orphaned.Subscribers)
	}
	if orphaned.Attribute(AttrSubscriptionNames) != "" {
		t.Errorf("%s names subscriptions it does not have: %q",
			seedOrphaned, orphaned.Attribute(AttrSubscriptionNames))
	}
}

/*
 * Depth is unknown rather than zero, on every topic, always.
 *
 * A topic stores nothing a caller can count. Zero would read as "this topic is
 * empty" where the truth is that there is no such number, and the seeded topic
 * holding twelve messages for two subscriptions is exactly the case that would
 * be reported wrongly.
 */
func TestLiveTopicsReportNoDepthRatherThanZero(t *testing.T) {
	conn := liveConn(t)

	topics, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(topics) == 0 {
		e2e.Missing(t, "the project holds no topics; run `npm run e2e:google-pubsub:seed`")
	}
	for _, topic := range topics {
		if topic.Depth != model.UnknownMetric {
			t.Errorf("%s reports a depth of %d; a topic holds nothing countable",
				topic.Ref.Name, topic.Depth)
		}
		if topic.Partitions != model.UnknownMetric {
			t.Errorf("%s reports %d partitions; a topic is not split",
				topic.Ref.Name, topic.Partitions)
		}
	}
}

// The prefix is applied by this driver rather than by the service, which has
// no filter of any kind - so it has to be asserted rather than assumed.
func TestLiveResourcePrefixNarrowsTheListing(t *testing.T) {
	requireProject(t)

	profile := liveProfile()
	profile.Options[OptionResourcePrefix] = seedOrders
	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	topics, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(topics) != 1 || topics[0].Ref.Name != seedOrders {
		names := make([]string, 0, len(topics))
		for _, topic := range topics {
			names = append(names, topic.Ref.Name)
		}
		t.Fatalf("prefix %q let through %v, want only %s", seedOrders, names, seedOrders)
	}

	// A topic's subscriber count is not narrowed with it. The prefix says what
	// to list; how many readers a topic has is a fact about the topic, and one
	// hidden reader is the difference between a topic nothing reads and one
	// three teams read.
	if topics[0].Subscribers != 2 {
		t.Errorf("%s reports %d subscriptions under a prefix that excludes them, want 2",
			seedOrders, topics[0].Subscribers)
	}
}

// One topic, read on its own rather than found in the listing: a project with
// a thousand topics should not answer for all of them to describe one.
func TestLiveDestinationDetailReadsOneTopic(t *testing.T) {
	conn := liveConn(t)

	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedQuiet})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed` (%v)", seedQuiet, err)
	}
	if topic.Ref.Name != seedQuiet {
		t.Errorf("detail named %q, want %q", topic.Ref.Name, seedQuiet)
	}
	if !strings.HasSuffix(topic.Attribute(AttrPath), "/topics/"+seedQuiet) {
		t.Errorf("path %q does not end in the topic name", topic.Attribute(AttrPath))
	}

	_, err = conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: "mqs-test-not-here"})
	if err == nil {
		t.Fatal("described a topic that does not exist")
	}
	if !strings.Contains(err.Error(), "mqs-test-not-here") {
		t.Errorf("error does not name the topic that is missing: %v", err)
	}
}

// createTopic makes a topic the test owns and removes it again.
func createTopic(t *testing.T, conn *Conn, spec TopicSpec) {
	t.Helper()
	if err := conn.CreateTopic(liveContext(t), spec); err != nil {
		t.Fatalf("create %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: spec.Name})
	})
}

// The whole round trip on the object this family has fewest settings for.
func TestLiveTopicCreateReadAndDelete(t *testing.T) {
	conn := liveConn(t)
	name := "mqs-test-topic-lifecycle"
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name})

	createTopic(t, conn, TopicSpec{
		Name:         name,
		RetentionSec: 1200,
		Labels:       map[string]string{"owner": "e2e"},
	})

	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if topic.Attribute(AttrRetentionSec) != "1200" {
		t.Errorf("retention = %q, want 1200", topic.Attribute(AttrRetentionSec))
	}
	if topic.Attribute(AttrLabelPrefix+"owner") != "e2e" {
		t.Errorf("label owner = %q, want e2e", topic.Attribute(AttrLabelPrefix+"owner"))
	}
	// A topic nothing subscribes to. Not an error and not an empty backlog:
	// every publish is accepted and discarded.
	if topic.Subscribers != 0 {
		t.Errorf("a topic just created reports %d subscriptions", topic.Subscribers)
	}

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name}); err == nil {
		t.Error("a deleted topic still describes itself")
	}
}

// Creating the same topic twice is refused, and the service's own message
// names neither the topic nor the project - which is unhelpful in a project
// several teams create topics in from a script.
func TestLiveCreateTopicRefusesADuplicateByName(t *testing.T) {
	conn := liveConn(t)
	name := "mqs-test-topic-duplicate"
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name})
	createTopic(t, conn, TopicSpec{Name: name})

	err := conn.CreateTopic(liveContext(t), TopicSpec{Name: name})
	if err == nil {
		t.Fatal("created the same topic twice")
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("error does not name the topic that already exists: %v", err)
	}
}

// Retention is the one setting a topic owns, and the bounds are the service's.
// They are checked before the request so the message can name the field the
// form actually draws rather than message_retention_duration.
func TestLiveUpdateTopicChangesRetention(t *testing.T) {
	conn := liveConn(t)
	name := "mqs-test-topic-retention"
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name})
	createTopic(t, conn, TopicSpec{Name: name, RetentionSec: 1200})

	if err := conn.UpdateTopic(liveContext(t), TopicSpec{Name: name, RetentionSec: 3600}); err != nil {
		t.Fatalf("update: %v", err)
	}
	topic, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if topic.Attribute(AttrRetentionSec) != "3600" {
		t.Errorf("retention = %q, want 3600", topic.Attribute(AttrRetentionSec))
	}

	if err := conn.UpdateTopic(liveContext(t), TopicSpec{Name: name, RetentionSec: 60}); err == nil {
		t.Error("accepted a retention below the service's own minimum")
	}
}

/*
 * What the emulator will not do, asserted rather than skipped.
 *
 * The real API takes `labels` in an UpdateTopic mask; the emulator refuses it
 * with INVALID_ARGUMENT saying labels is not a known Topic field. So a user
 * editing a label against an emulator gets an error and the same edit works
 * against a real project - which is worth recording here rather than leaving
 * as a gap somebody rediscovers.
 *
 * If this ever starts passing, the emulator has grown the field and the
 * assertion should become the ordinary one.
 */
func TestLiveEmulatorRefusesATopicLabelUpdate(t *testing.T) {
	conn := liveConn(t)
	if conn.Emulator() == "" {
		e2e.Missing(t, "this asserts an emulator limitation and the connection is not one")
	}
	name := "mqs-test-topic-labels"
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name})
	createTopic(t, conn, TopicSpec{Name: name, Labels: map[string]string{"owner": "e2e"}})

	err := conn.UpdateTopic(liveContext(t), TopicSpec{
		Name:   name,
		Labels: map[string]string{"owner": "someone-else"},
	})
	if err == nil {
		t.Fatal("the emulator accepted a label update, which it did not use to; " +
			"drop this test and let the ordinary update path cover it")
	}
	if !strings.Contains(err.Error(), "labels") {
		t.Errorf("the refusal is not the known one about labels: %v", err)
	}
}

func findSubscription(list []*model.Subscription, name string) *model.Subscription {
	for _, entry := range list {
		if entry.Ref.Name == name {
			return entry
		}
	}
	return nil
}

// createSubscription makes a subscription the test owns and removes it again.
func createSubscription(t *testing.T, conn *Conn, spec SubscriptionSpec) {
	t.Helper()
	if err := conn.CreateSubscriptionFrom(liveContext(t), spec); err != nil {
		t.Fatalf("create %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveSubscription(context.Background(), model.SubscriptionRef{Name: spec.Name})
	})
}

/*
 * The concept this family adds, read end to end.
 *
 * Two subscriptions on one topic, each its own object with its own settings.
 * That is what an SQS queue could not express at all: there, a consumer was
 * whoever called ReceiveMessage and every reader shared one backlog.
 */
func TestLiveListSubscriptionsReadsEachAsItsOwnObject(t *testing.T) {
	conn := liveConn(t)

	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	worker := findSubscription(subscriptions, seedWorker)
	audit := findSubscription(subscriptions, seedAudit)
	if worker == nil || audit == nil {
		e2e.Missing(t, "%s and %s are not seeded; run `npm run e2e:google-pubsub:seed`",
			seedWorker, seedAudit)
	}

	// Same topic, different settings: the two are separate objects rather than
	// two views of one.
	if worker.Attribute(SubAttrTopic) != seedOrders || audit.Attribute(SubAttrTopic) != seedOrders {
		t.Errorf("the two subscriptions read %q and %q, want both on %s",
			worker.Attribute(SubAttrTopic), audit.Attribute(SubAttrTopic), seedOrders)
	}
	if worker.Attribute(SubAttrAckDeadlineSec) == audit.Attribute(SubAttrAckDeadlineSec) {
		t.Errorf("both report an ack deadline of %q; the seed gives them different ones",
			worker.Attribute(SubAttrAckDeadlineSec))
	}
	if worker.Attribute(SubAttrDeadLetterTopic) != seedDeadLetters {
		t.Errorf("%s dead-letters into %q, want %s",
			seedWorker, worker.Attribute(SubAttrDeadLetterTopic), seedDeadLetters)
	}
	if audit.Attribute(SubAttrDeadLetterTopic) != "" {
		t.Errorf("%s reports a dead-letter topic it was not given: %q",
			seedAudit, audit.Attribute(SubAttrDeadLetterTopic))
	}
	if worker.Attribute(SubAttrRetryMinSec) != "10" || worker.Attribute(SubAttrRetryMaxSec) != "600" {
		t.Errorf("%s reports a retry policy of %q..%q, want 10..600",
			seedWorker, worker.Attribute(SubAttrRetryMinSec), worker.Attribute(SubAttrRetryMaxSec))
	}
	if worker.Attribute(SubAttrDelivery) != DeliveryPull {
		t.Errorf("%s reports delivery %q, want %q",
			seedWorker, worker.Attribute(SubAttrDelivery), DeliveryPull)
	}
}

/*
 * The backlog is never a number, and that is the decision this family turns on.
 *
 * num_undelivered_messages is a Cloud Monitoring metric. It is not on the
 * Subscription the admin API returns, and the emulator serves no Monitoring at
 * all - so the honest answer is "unknown with a reason" rather than a figure.
 * The seeded subscription is holding twelve messages, which is exactly the case
 * a zero would report wrongly.
 */
func TestLiveSubscriptionBacklogIsUnknownRatherThanZero(t *testing.T) {
	conn := liveConn(t)

	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(subscriptions) == 0 {
		e2e.Missing(t, "the project holds no subscriptions; run `npm run e2e:google-pubsub:seed`")
	}
	for _, subscription := range subscriptions {
		if subscription.Backlog != model.UnknownMetric {
			t.Errorf("%s reports a backlog of %d; the admin API reports none",
				subscription.Ref.Name, subscription.Backlog)
		}
		if subscription.Members != model.UnknownMetric {
			t.Errorf("%s reports %d members; nothing registers as a consumer",
				subscription.Ref.Name, subscription.Members)
		}
	}

	reason, degraded := conn.Capabilities().DegradedReason(model.CapSubscriptionLag)
	if !degraded || reason != lagInMonitoring {
		t.Errorf("the backlog is unknown and the connection does not explain why: %q", reason)
	}
}

// The whole round trip on the object this family adds.
func TestLiveSubscriptionCreateReadUpdateAndDelete(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-sub-topic"
	dead := "mqs-test-sub-dead"
	name := "mqs-test-sub-lifecycle"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: dead})
	createTopic(t, conn, TopicSpec{Name: topic})
	createTopic(t, conn, TopicSpec{Name: dead})

	createSubscription(t, conn, SubscriptionSpec{
		Name:               name,
		Topic:              topic,
		AckDeadlineSec:     30,
		RetentionSec:       3600,
		RetainAcked:        true,
		Filter:             `attributes.kind = "order"`,
		DeadLetterTopic:    dead,
		MaxAttempts:        7,
		RetryMinBackoffSec: 15,
		RetryMaxBackoffSec: 120,
	})

	read, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: name})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	for key, want := range map[string]string{
		SubAttrTopic:           topic,
		SubAttrAckDeadlineSec:  "30",
		SubAttrRetentionSec:    "3600",
		SubAttrRetainAcked:     "true",
		SubAttrFilter:          `attributes.kind = "order"`,
		SubAttrDeadLetterTopic: dead,
		SubAttrMaxAttempts:     "7",
		SubAttrRetryMinSec:     "15",
		SubAttrRetryMaxSec:     "120",
	} {
		if got := read.Attribute(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	// An update writes only what it is given: the mask is built from the spec,
	// so the retention set above has to survive a change to the ack deadline.
	if err := conn.UpdateSubscriptionFrom(liveContext(t), SubscriptionSpec{
		Name:            name,
		AckDeadlineSec:  60,
		RetainAcked:     true,
		DeadLetterTopic: dead,
		MaxAttempts:     7,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	read, err = conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: name})
	if err != nil {
		t.Fatalf("detail after update: %v", err)
	}
	if read.Attribute(SubAttrAckDeadlineSec) != "60" {
		t.Errorf("ack deadline = %q, want 60", read.Attribute(SubAttrAckDeadlineSec))
	}
	if read.Attribute(SubAttrRetentionSec) != "3600" {
		t.Errorf("retention = %q after an update that did not mention it, want 3600",
			read.Attribute(SubAttrRetentionSec))
	}

	// An empty dead-letter topic is how the policy is removed. There is no
	// separate call, and leaving the field out would keep it instead.
	if err := conn.UpdateSubscriptionFrom(liveContext(t), SubscriptionSpec{Name: name}); err != nil {
		t.Fatalf("clearing the dead-letter policy: %v", err)
	}
	read, err = conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: name})
	if err != nil {
		t.Fatalf("detail after clearing: %v", err)
	}
	if read.Attribute(SubAttrDeadLetterTopic) != "" {
		t.Errorf("the dead-letter policy survived being cleared: %q",
			read.Attribute(SubAttrDeadLetterTopic))
	}

	if err := conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: name}); err == nil {
		t.Error("a deleted subscription still describes itself")
	}
}

// A subscription needs a topic that exists, and the service's own refusal
// names the subscription rather than the topic that is actually missing.
func TestLiveCreateSubscriptionRefusesAMissingTopic(t *testing.T) {
	conn := liveConn(t)

	err := conn.CreateSubscriptionFrom(liveContext(t), SubscriptionSpec{
		Name:  "mqs-test-sub-orphan",
		Topic: "mqs-test-no-such-topic",
	})
	if err == nil {
		_ = conn.RemoveSubscription(liveContext(t),
			model.SubscriptionRef{Name: "mqs-test-sub-orphan"})
		t.Fatal("created a subscription on a topic that does not exist")
	}
	if !strings.Contains(err.Error(), "mqs-test-no-such-topic") {
		t.Errorf("error does not name the topic that is missing: %v", err)
	}
}

/*
 * Deleting a topic leaves its subscriptions behind, and this is the whole of
 * what makes that worth showing.
 *
 * The subscription survives, reports its topic as the service's own
 * _deleted-topic_ marker, and can never receive another message. It is still
 * billed for what it holds, and nothing else in the app would say so.
 */
func TestLiveASubscriptionOutlivesItsTopic(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-doomed-topic"
	name := "mqs-test-orphaned-sub"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})

	createTopic(t, conn, TopicSpec{Name: topic})
	createSubscription(t, conn, SubscriptionSpec{Name: name, Topic: topic})

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic}); err != nil {
		t.Fatalf("delete topic: %v", err)
	}

	read, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: name})
	if err != nil {
		t.Fatalf("the subscription went with its topic, which the service does not do: %v", err)
	}
	if read.Attribute(SubAttrTopic) != deletedTopic {
		t.Errorf("topic = %q, want the service's own %q marker",
			read.Attribute(SubAttrTopic), deletedTopic)
	}
	if read.Status != model.SubscriptionOffline {
		t.Errorf("status = %q; a subscription that can never receive again is offline", read.Status)
	}
	if read.Destinations != 0 {
		t.Errorf("it reports %d destinations, and its topic is gone", read.Destinations)
	}
}

/*
 * Seek, both halves, against a real subscription.
 *
 * A snapshot names the place itself and a timestamp names a moment. Both work
 * here, which took a correction to find out: the emulator answers a
 * seek-to-time with Unimplemented, and the hole is one subscription's setting
 * rather than the endpoint's - the next test is what pins which.
 */
func TestLiveSeekMovesASubscriptionBothWays(t *testing.T) {
	conn := liveConn(t)

	topic := "mqs-test-seek-topic"
	name := "mqs-test-seek-sub"
	snapshot := "mqs-test-seek-snapshot"
	_ = conn.RemoveSnapshot(liveContext(t), snapshot)
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	createTopic(t, conn, TopicSpec{Name: topic})
	createSubscription(t, conn, SubscriptionSpec{Name: name, Topic: topic, RetainAcked: true})

	if err := conn.CreateSnapshot(liveContext(t), snapshot, name); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	t.Cleanup(func() { _ = conn.RemoveSnapshot(context.Background(), snapshot) })

	snapshots, err := conn.ListSnapshots(liveContext(t))
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	var taken *Snapshot
	for _, entry := range snapshots {
		if entry.Name == snapshot {
			taken = entry
		}
	}
	if taken == nil {
		t.Fatalf("the snapshot just taken is not listed: %v", snapshots)
	}
	// A snapshot belongs to the topic rather than to the subscription it came
	// from, which is what lets a second reader be sought to it.
	if taken.Topic != topic {
		t.Errorf("snapshot topic = %q, want %q", taken.Topic, topic)
	}
	if taken.ExpiresAt <= time.Now().UnixMilli() {
		t.Errorf("the snapshot is already expired: %d", taken.ExpiresAt)
	}

	if err := conn.SetSubscriptionPosition(liveContext(t), model.PositionRequest{
		Ref:      model.SubscriptionRef{Name: name},
		Position: snapshot,
	}); err != nil {
		t.Fatalf("seek to a snapshot: %v", err)
	}

	if err := conn.ResetOffset(liveContext(t), model.ResetOffsetRequest{
		Group:     name,
		Timestamp: time.Now().Add(-time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("seek to a moment: %v", err)
	}

	// A reset with no moment in it is refused here rather than sent, because
	// the service would take the zero time and discard the whole backlog.
	if err := conn.ResetOffset(liveContext(t), model.ResetOffsetRequest{Group: name}); err == nil {
		t.Error("accepted a reset naming no moment at all")
	}
}

/*
 * What the emulator will not do, narrowed to what it actually is.
 *
 * Seeking to a timestamp answers Unimplemented for a subscription created with
 * message ordering on, and works for every other one. That is one
 * subscription's setting rather than anything about the endpoint, so the
 * capability stays declared and the refusal is an error at the call - with a
 * message naming ordering, because the service's own is the bare word
 * "Unimplemented".
 *
 * If this ever starts passing, the emulator has closed the hole and the
 * special case in ResetOffset can go.
 */
func TestLiveEmulatorWillNotSeekAnOrderedSubscriptionToATime(t *testing.T) {
	conn := liveConn(t)
	if conn.Emulator() == "" {
		e2e.Missing(t, "this asserts an emulator limitation and the connection is not one")
	}

	topic := "mqs-test-ordered-topic"
	name := "mqs-test-ordered-sub"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	createTopic(t, conn, TopicSpec{Name: topic})
	createSubscription(t, conn, SubscriptionSpec{Name: name, Topic: topic, Ordering: true})

	err := conn.ResetOffset(liveContext(t), model.ResetOffsetRequest{
		Group:     name,
		Timestamp: time.Now().Add(-time.Hour).UnixMilli(),
	})
	if err == nil {
		t.Fatal("the emulator seeked an ordered subscription to a time, which it did not use to; " +
			"drop the special case in ResetOffset and let the ordinary path cover it")
	}
	if !strings.Contains(err.Error(), "ordering") {
		t.Errorf("the refusal does not name the setting behind it: %v", err)
	}

	// The other target still works on the same subscription, which is what
	// makes the message worth its length: there is something else to do.
	snapshot := "mqs-test-ordered-snapshot"
	_ = conn.RemoveSnapshot(liveContext(t), snapshot)
	if err := conn.CreateSnapshot(liveContext(t), snapshot, name); err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	t.Cleanup(func() { _ = conn.RemoveSnapshot(context.Background(), snapshot) })
	if err := conn.SetSubscriptionPosition(liveContext(t), model.PositionRequest{
		Ref:      model.SubscriptionRef{Name: name},
		Position: snapshot,
	}); err != nil {
		t.Errorf("seeking an ordered subscription to a snapshot: %v", err)
	}
}

// A snapshot is a restore point, and leaving one lying around keeps the topic
// holding everything it could restore - so it has to be removable.
func TestLiveSnapshotCreateAndDelete(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-snap-topic"
	name := "mqs-test-snap-sub"
	snapshot := "mqs-test-snap-point"
	_ = conn.RemoveSnapshot(liveContext(t), snapshot)
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	createTopic(t, conn, TopicSpec{Name: topic})
	createSubscription(t, conn, SubscriptionSpec{Name: name, Topic: topic})

	if err := conn.CreateSnapshot(liveContext(t), snapshot, name); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := conn.CreateSnapshot(liveContext(t), snapshot, name); err == nil {
		_ = conn.RemoveSnapshot(liveContext(t), snapshot)
		t.Fatal("took the same snapshot twice")
	}
	if err := conn.RemoveSnapshot(liveContext(t), snapshot); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := conn.RemoveSnapshot(liveContext(t), snapshot); err == nil {
		t.Error("deleted a snapshot that was already gone")
	}
}

/*
 * Browsing reads a subscription, not a topic, and the difference is the whole
 * of what this family's message page had to be rebuilt for.
 *
 * The topic holds nothing. Two subscriptions on one topic hold different
 * messages, and there is no third place with all of them - so the page offers
 * subscriptions and this call takes one.
 */
func TestLiveQueryMessagesReadsASubscription(t *testing.T) {
	conn := liveConn(t)

	items, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      seedAudit,
		MaxResults: 20,
	})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed` (%v)", seedAudit, err)
	}
	if len(items) != 12 {
		t.Fatalf("browsed %d messages, want the 12 the seed publishes", len(items))
	}
	for _, item := range items {
		if item.Topic != seedAudit {
			t.Errorf("message reports %q, want the subscription it was read from", item.Topic)
		}
		if item.Body == "" {
			t.Errorf("message %s has an empty body", item.MessageID)
		}
		if item.StoreTimestamp <= 0 {
			t.Errorf("message %s carries no publish time", item.MessageID)
		}
		if item.Properties[PropAttributePrefix+"kind"] != "order" {
			t.Errorf("message %s lost the attribute the seed set: %v",
				item.MessageID, item.Properties)
		}
	}
	// Newest first, which is this app's ordering rather than the service's:
	// Pull returns whatever its servers offered, in no order at all.
	for index := 1; index < len(items); index++ {
		if items[index-1].StoreTimestamp < items[index].StoreTimestamp {
			t.Fatalf("messages are not newest first at %d", index)
		}
	}

	// A browse names a topic by mistake often enough to be worth a message
	// that says so rather than a NotFound naming a subscription path.
	_, err = conn.QueryMessages(liveContext(t), model.MessageQueryParams{Topic: seedOrders})
	if err == nil {
		t.Error("browsed a topic name as though it were a subscription")
	}
}

// publishForTest puts messages on a topic the test owns.
//
// It goes through the client rather than the driver's own send path on
// purpose: this is arranging a fixture, and a test that browsed the seed's
// messages instead would spend their delivery attempts - five of them, after
// which the seed dead-letters itself and every later run sees a different
// project.
func publishForTest(t *testing.T, conn *Conn, topic string, count int) {
	t.Helper()
	publisher := conn.client.Publisher(conn.topicPath(topic))
	defer publisher.Stop()
	for index := 1; index <= count; index++ {
		result := publisher.Publish(liveContext(t), &pubsub.Message{
			Data:       []byte(fmt.Sprintf("browse-%d", index)),
			Attributes: map[string]string{"kind": "browse"},
		})
		if _, err := result.Get(liveContext(t)); err != nil {
			t.Fatalf("publish to %s: %v", topic, err)
		}
	}
}

/*
 * The caveat, exercised rather than asserted.
 *
 * A browse is a delivery: the delivery attempt goes up and does not come back
 * down, which is what counts towards the dead-letter policy. Browsing the same
 * subscription twice has to show that - and it also has to show the release
 * working, because a browse that did not hand its messages back would find
 * nothing the second time.
 *
 * On a topic the test owns, because proving the caveat means spending
 * delivery attempts and the seed only has five of them.
 */
func TestLiveBrowsingDeliversAndHandsBack(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-browse-topic"
	dead := "mqs-test-browse-dead"
	name := "mqs-test-browse-sub"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: dead})
	createTopic(t, conn, TopicSpec{Name: topic})
	createTopic(t, conn, TopicSpec{Name: dead})
	createSubscription(t, conn, SubscriptionSpec{
		Name: name, Topic: topic, AckDeadlineSec: 60,
		DeadLetterTopic: dead, MaxAttempts: 50,
	})
	publishForTest(t, conn, topic, 3)

	first, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      name,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("first browse: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("browsed %d messages, want 3", len(first))
	}

	attempts := map[string]int{}
	for _, item := range first {
		attempt, err := strconv.Atoi(item.Properties[PropDeliveryAttempt])
		if err != nil {
			t.Fatalf("message %s reports no delivery attempt, and its subscription has a "+
				"dead-letter policy: %v", item.MessageID, item.Properties)
		}
		attempts[item.MessageID] = attempt
	}

	// The release is what makes this possible at all: without handing the
	// first batch back, the second browse would see nothing until the ack
	// deadline expired a minute later.
	second, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      name,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("second browse: %v", err)
	}
	if len(second) != 3 {
		t.Fatalf("the second browse found %d messages; the first did not hand its own back",
			len(second))
	}

	raised := 0
	for _, item := range second {
		attempt, err := strconv.Atoi(item.Properties[PropDeliveryAttempt])
		if err != nil {
			continue
		}
		if before, seen := attempts[item.MessageID]; seen && attempt > before {
			raised++
		}
	}
	if raised != len(second) {
		t.Errorf("browsing twice raised %d of %d delivery attempts, and the caveat says it "+
			"raises every one; if the service stopped counting a browse, the caveat should go too",
			raised, len(second))
	}
}

// An empty subscription answers empty rather than holding the request open,
// which is what the per-call window is for: Pub/Sub keeps an unsatisfied pull
// waiting, and the emulator waits until just short of the caller's deadline.
func TestLiveBrowsingAnEmptySubscriptionReturnsPromptly(t *testing.T) {
	conn := liveConn(t)

	started := time.Now()
	items, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      seedIdle,
		MaxResults: 10,
	})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed` (%v)", seedIdle, err)
	}
	if len(items) != 0 {
		t.Errorf("an idle subscription answered with %d messages", len(items))
	}
	// The window is two seconds and the caller's deadline is sixty. Anything
	// close to the second number means the window stopped being applied.
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("browsing an empty subscription took %v; the per-call window is not being applied",
			elapsed.Round(time.Second))
	}
}

/*
 * Publishing, and the thing about it that no other family here shares.
 *
 * A topic stores nothing. The publish reaches whatever subscriptions exist at
 * that instant and is discarded if none do, and the service reports success
 * either way - so a send to an unsubscribed topic is accepted, acknowledged
 * and thrown away with nothing anywhere recording that it happened.
 */
func TestLivePublishReachesEverySubscriptionAndNoneAtAll(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-publish-topic"
	first := "mqs-test-publish-a"
	second := "mqs-test-publish-b"
	lonely := "mqs-test-publish-lonely"
	for _, name := range []string{first, second} {
		_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	}
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: lonely})
	createTopic(t, conn, TopicSpec{Name: topic})
	createTopic(t, conn, TopicSpec{Name: lonely})
	createSubscription(t, conn, SubscriptionSpec{Name: first, Topic: topic, AckDeadlineSec: 60})
	createSubscription(t, conn, SubscriptionSpec{Name: second, Topic: topic, AckDeadlineSec: 60})

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Topic:      topic,
		Body:       "publish-fan-out",
		Count:      3,
		Attributes: map[string]string{"kind": "test"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if result.Sent != 3 {
		t.Errorf("sent = %d, want 3", result.Sent)
	}
	if result.MessageID == "" {
		t.Error("the send reports no message id at all")
	}

	// Every subscription gets its own copy: three messages published, three
	// waiting on each of two subscriptions.
	for _, name := range []string{first, second} {
		items, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
			Topic:      name,
			MaxResults: 10,
		})
		if err != nil {
			t.Fatalf("browse %s: %v", name, err)
		}
		if len(items) != 3 {
			t.Errorf("%s holds %d messages, want its own copy of all 3", name, len(items))
		}
		for _, item := range items {
			if item.Properties[PropAttributePrefix+"kind"] != "test" {
				t.Errorf("%s lost the attribute the send set: %v", name, item.Properties)
			}
		}
	}

	// And the other half: a topic nothing subscribes to accepts the message
	// and reports success, and there is nowhere afterwards it can be found.
	discarded, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: lonely,
		Body:  "into-the-void",
	})
	if err != nil {
		t.Fatalf("publish to an unsubscribed topic: %v", err)
	}
	if discarded.Sent != 1 {
		t.Errorf("sent = %d to a topic nothing reads, want the service's own success", discarded.Sent)
	}
}

// An ordering key needs the publisher to expect one, and the client's own
// refusal names an internal flag rather than the field a console draws.
func TestLivePublishCarriesAnOrderingKey(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-ordering-topic"
	name := "mqs-test-ordering-reader"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	createTopic(t, conn, TopicSpec{Name: topic})
	createSubscription(t, conn, SubscriptionSpec{
		Name: name, Topic: topic, AckDeadlineSec: 60, Ordering: true,
	})

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic:       topic,
		Body:        "ordered",
		Count:       2,
		OrderingKey: "customer-1",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	items, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      name,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("browsed %d messages, want 2", len(items))
	}
	for _, item := range items {
		if item.Properties[PropOrderingKey] != "customer-1" {
			t.Errorf("message %s lost its ordering key: %v", item.MessageID, item.Properties)
		}
		// The canonical keys field is where a reader groups by, which is the
		// nearest thing RocketMQ's has to an ordering key.
		if item.Keys != "customer-1" {
			t.Errorf("message %s reports keys %q, want the ordering key", item.MessageID, item.Keys)
		}
	}
}

/*
 * What the canonical send port cannot carry, refused rather than dropped.
 *
 * MessagePublisher's signature is RocketMQ's: a topic, tags, keys and a delay
 * level. Pub/Sub has the topic and the key; a tag would be silently discarded,
 * and a delay would be reported as holding a message back that went out at
 * once - the service has no way to hold one at all.
 */
func TestLiveSendMessageRefusesWhatPubSubHasNoConceptOf(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-send-topic"
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	createTopic(t, conn, TopicSpec{Name: topic})

	if _, err := conn.SendMessage(liveContext(t), topic, "orders", "", "body", 0); err == nil {
		t.Error("accepted a tag, which a pub/sub message cannot carry")
	}
	if _, err := conn.SendMessage(liveContext(t), topic, "", "", "body", 30); err == nil {
		t.Error("accepted a delay, and pub/sub cannot hold a message back")
	}

	id, err := conn.SendMessage(liveContext(t), topic, "", "", "plain", 0)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id == "" {
		t.Error("the send reports no message id")
	}

	// A message with neither a body nor an attribute is refused here, because
	// the service's own refusal names no field at all.
	if _, err := conn.Publish(liveContext(t), PublishRequest{Topic: topic}); err == nil {
		t.Error("accepted a message with no body and no attributes")
	}
	// One with only attributes is fine, and is how a filtered subscription is
	// exercised without a payload.
	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic:      topic,
		Attributes: map[string]string{"kind": "signal"},
	}); err != nil {
		t.Errorf("refused a message carrying only attributes: %v", err)
	}
}

/*
 * The dead-letter topology, walked backwards.
 *
 * Nothing marks a dead-letter topic: it is an ordinary topic that some
 * subscription's policy points at. And the policy is on the subscription
 * rather than on the topic, which is what makes a source two names - the topic
 * a message came from, and the subscription that stopped trying.
 */
func TestLiveDeadLetterQueuesNameBothHalvesOfEverySource(t *testing.T) {
	conn := liveConn(t)

	queues, err := conn.DeadLetterQueues(liveContext(t), "")
	if err != nil {
		t.Fatalf("dead letters: %v", err)
	}

	var found *model.DeadLetterQueue
	for _, queue := range queues {
		if queue.Name == seedDeadLetters {
			found = queue
		}
	}
	if found == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:google-pubsub:seed`", seedDeadLetters)
	}

	if len(found.Sources) != 1 {
		t.Fatalf("%s reports %d sources, want the one the seed creates: %v",
			seedDeadLetters, len(found.Sources), found.Sources)
	}
	source := found.Sources[0]
	if source.Queue != seedOrders {
		t.Errorf("source topic = %q, want %q", source.Queue, seedOrders)
	}
	// The half a RabbitMQ source cannot carry, and the half that matters here:
	// which reader of that topic gave up.
	if source.Subscription != seedWorker {
		t.Errorf("source subscription = %q, want %q", source.Subscription, seedWorker)
	}
	// Nothing is re-published through a routing layer - the service moves the
	// message itself - so neither of RabbitMQ's fields has a counterpart.
	if source.Exchange != "" || source.RoutingKey != "" {
		t.Errorf("source reports an exchange %q and routing key %q, and Pub/Sub has neither",
			source.Exchange, source.RoutingKey)
	}

	// A topic holds nothing countable, dead letters included: zero would read
	// as "this backlog has been dealt with".
	if found.Depth != model.UnknownMetric {
		t.Errorf("%s reports a depth of %d; a topic holds nothing countable",
			seedDeadLetters, found.Depth)
	}
	// The seed puts a reader on it, which is what separates a dead-letter
	// topic somebody is watching from one that discards on arrival.
	if found.Consumers != 1 {
		t.Errorf("%s reports %d subscriptions, want the one the seed creates",
			seedDeadLetters, found.Consumers)
	}
}

/*
 * A dead-letter topic with no subscription, which is the failure this page
 * exists to find.
 *
 * A topic stores nothing. A dead letter published to one nothing subscribes to
 * is discarded on arrival - and the messages a system gave up on are exactly
 * the ones nobody notices disappearing, because there is no backlog anywhere
 * to notice them by.
 */
func TestLiveADeadLetterTopicWithNoReaderIsReportedAsSuch(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-dl-source"
	dead := "mqs-test-dl-sink"
	name := "mqs-test-dl-worker"
	_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: dead})
	createTopic(t, conn, TopicSpec{Name: topic})
	createTopic(t, conn, TopicSpec{Name: dead})
	createSubscription(t, conn, SubscriptionSpec{
		Name: name, Topic: topic, DeadLetterTopic: dead, MaxAttempts: 5,
	})

	queues, err := conn.DeadLetterQueues(liveContext(t), "")
	if err != nil {
		t.Fatalf("dead letters: %v", err)
	}
	var found *model.DeadLetterQueue
	for _, queue := range queues {
		if queue.Name == dead {
			found = queue
		}
	}
	if found == nil {
		t.Fatalf("a topic %s dead-letters into is not listed at all", name)
	}
	if found.Consumers != 0 {
		t.Errorf("%s reports %d subscriptions, and nothing was created on it", dead, found.Consumers)
	}
	if len(found.Sources) != 1 || found.Sources[0].Subscription != name {
		t.Errorf("%s reports sources %v, want just %s", dead, found.Sources, name)
	}
}

/*
 * A message that runs out of delivery attempts really does move, and it is
 * worth exercising rather than assuming: the count includes this app's own
 * browse, which is the whole reason the browse carries a caveat.
 */
func TestLiveAMessageOutOfAttemptsReachesTheDeadLetterTopic(t *testing.T) {
	conn := liveConn(t)
	topic := "mqs-test-giveup-topic"
	dead := "mqs-test-giveup-dead"
	worker := "mqs-test-giveup-worker"
	reader := "mqs-test-giveup-reader"
	for _, name := range []string{worker, reader} {
		_ = conn.RemoveSubscription(liveContext(t), model.SubscriptionRef{Name: name})
	}
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: topic})
	_ = conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: dead})
	createTopic(t, conn, TopicSpec{Name: topic})
	createTopic(t, conn, TopicSpec{Name: dead})
	createSubscription(t, conn, SubscriptionSpec{
		Name: worker, Topic: topic, AckDeadlineSec: 10,
		DeadLetterTopic: dead, MaxAttempts: minDeliveryAttempts,
	})
	createSubscription(t, conn, SubscriptionSpec{Name: reader, Topic: dead, AckDeadlineSec: 60})

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Topic: topic,
		Body:  "gives-up",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Each browse is a delivery. After the policy's limit the service moves
	// the message, which is exactly what the caveat warns about.
	for attempt := 0; attempt < minDeliveryAttempts+1; attempt++ {
		if _, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
			Topic:      worker,
			MaxResults: 5,
		}); err != nil {
			t.Fatalf("browse %d: %v", attempt, err)
		}
	}

	moved, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      reader,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("browse the dead-letter topic: %v", err)
	}
	if len(moved) == 0 {
		t.Fatal("browsing past the delivery limit did not move the message, and the browse " +
			"caveat says it counts towards exactly that")
	}
	if moved[0].Body != "gives-up" {
		t.Errorf("the dead-letter topic holds %q, want the message that ran out of attempts",
			moved[0].Body)
	}
}
