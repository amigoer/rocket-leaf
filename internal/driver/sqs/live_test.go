package sqs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live environment, as tests/e2e/sqs/compose.yaml publishes it.
//
// LocalStack, reached through the endpoint override. That is not a testing
// shortcut around the real code path: a VPC interface endpoint is configured
// with the same field, so every test here exercises it.
const (
	liveEndpoint  = "http://127.0.0.1:4566"
	liveRegion    = "eu-west-1"
	liveAccessKey = "test"
	liveSecretKey = "test"
)

// The queues scripts/e2e-sqs-seed.sh creates. Tests read these and never write
// to them; anything a test needs to change it creates for itself.
const (
	seedOrders  = "MQS-SEED-orders"
	seedDLQ     = "MQS-SEED-orders-dlq"
	seedDelayed = "MQS-SEED-delayed"
	seedEmpty   = "MQS-SEED-empty"
	seedFIFO    = "MQS-SEED-orders.fifo"
)

func requireRegion(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.SQS,
		Name:   "the sqs environment",
		Start:  "npm run e2e:sqs:up",
		Probe:  e2e.HTTPGet(liveEndpoint + "/_localstack/health"),
	})
}

// liveProfile is the environment as a user would configure it: a region, a
// credential, and no address anywhere on the form.
func liveProfile() model.ConnectionProfile {
	profile := model.ConnectionProfile{
		ID:         1,
		Name:       "sqs e2e",
		Kind:       model.KindSQS,
		TimeoutSec: 10,
		Auth:       model.AuthConfig{Mechanism: model.AuthPlain},
		Options: map[string]string{
			OptionRegion:      liveRegion,
			OptionEndpointURL: liveEndpoint,
		},
	}
	profile.SetSecret(SecretAccessKeyID, liveAccessKey)
	profile.SetSecret(SecretSecretAccessKey, liveSecretKey)
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
	requireRegion(t)
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
 * A profile whose Endpoints is empty opens, which nothing else in this app can
 * do. It is asserted here rather than only in the descriptor test because the
 * descriptor only says the form asks for no address - this proves the driver
 * needs none either.
 */
func TestLiveOpenNeedsNoAddress(t *testing.T) {
	requireRegion(t)

	profile := liveProfile()
	if profile.Endpoints != "" {
		t.Fatalf("the live profile carries an address %q; this family has none", profile.Endpoints)
	}

	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	if conn.Region() != liveRegion {
		t.Errorf("region = %q, want %q", conn.Region(), liveRegion)
	}
	if err := conn.Ping(liveContext(t)); err != nil {
		t.Errorf("ping: %v", err)
	}
}

// Nothing is dialled, so a wrong region, a wrong credential and a wrong
// endpoint all look identical until a request is signed and sent. Open has to
// send one, or every one of them opens and then reports an empty account.
func TestLiveOpenProvesTheCredentialReachesSQS(t *testing.T) {
	requireRegion(t)

	t.Run("an endpoint with nothing behind it", func(t *testing.T) {
		profile := liveProfile()
		profile.TimeoutSec = 2
		profile.Options[OptionEndpointURL] = "http://127.0.0.1:4599"
		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened against an endpoint that answers nothing")
		}
	})

	t.Run("a profile naming no region", func(t *testing.T) {
		profile := liveProfile()
		delete(profile.Options, OptionRegion)
		err := func() error { _, err := open(liveContext(t), profile); return err }()
		if err == nil {
			t.Fatal("opened with no region to sign for")
		}
		if !strings.Contains(err.Error(), "region") {
			t.Errorf("error does not name the missing field: %v", err)
		}
	})
}

// Close is called on disconnect and again on shutdown, so the second call has
// to be the one that does nothing.
func TestLiveCloseIsIdempotent(t *testing.T) {
	requireRegion(t)

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

// The name a user types has to reach the URL every call actually takes, and a
// name nothing answers for has to say so rather than failing later with an
// SDK error naming an operation the user never asked for.
func TestLiveQueueURLResolvesANameAndRefusesAnUnknownOne(t *testing.T) {
	conn := liveConn(t)

	url, err := conn.queueURL(liveContext(t), seedOrders)
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:sqs:seed` (%v)", seedOrders, err)
	}
	if !strings.HasSuffix(url, "/"+seedOrders) {
		t.Errorf("queue url %q does not end in the queue name", url)
	}
	if queueNameOf(url) != seedOrders {
		t.Errorf("queueNameOf(%q) = %q, want %q", url, queueNameOf(url), seedOrders)
	}

	_, err = conn.queueURL(liveContext(t), "MQS-TEST-not-here")
	if err == nil {
		t.Fatal("resolved a queue that does not exist")
	}
	if !strings.Contains(err.Error(), "MQS-TEST-not-here") {
		t.Errorf("error does not name the queue that is missing: %v", err)
	}
}

// attrInt reads one of the driver's own numeric attributes off a row.
func attrInt(t *testing.T, destination *model.Destination, key string) int64 {
	t.Helper()
	parsed, err := strconv.ParseInt(destination.Attribute(key), 10, 64)
	if err != nil {
		t.Fatalf("%s reported %s=%q, which is not a number", destination.Ref.Name, key, destination.Attribute(key))
	}
	return parsed
}

func destinationNamed(destinations []*model.Destination, name string) *model.Destination {
	for _, destination := range destinations {
		if destination.Ref.Name == name {
			return destination
		}
	}
	return nil
}

/*
 * The listing is two calls per board and the second one is per queue, so this
 * asserts what the fold produces rather than only that it produced something:
 * the three kinds of held message, the redrive target read out of a policy
 * that only carries an ARN, and the FIFO flag read off the name.
 */
func TestLiveListDestinationsReadsEveryQueuesFigures(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	orders := destinationNamed(destinations, seedOrders)
	if orders == nil {
		e2e.Missing(t, "%s is not in the listing; run `npm run e2e:sqs:seed`", seedOrders)
	}
	if orders.Depth != 12 {
		t.Errorf("%s depth = %d, want the 12 the seed sent", seedOrders, orders.Depth)
	}
	// Visible plus in flight rather than visible alone: another suite reading
	// this region can be holding some of them at this instant, and a message
	// in flight is still one the queue is holding.
	if held := attrInt(t, orders, AttrVisible) + attrInt(t, orders, AttrInFlight); held != 12 {
		t.Errorf("%s holds %d visible and in flight, want the 12 the seed sent", seedOrders, held)
	}
	if got := orders.Attribute(AttrDeadLetterQueue); got != seedDLQ {
		t.Errorf("%s dead-letter queue = %q, want %q", seedOrders, got, seedDLQ)
	}
	if got := orders.Attribute(AttrMaxReceiveCount); got != "3" {
		t.Errorf("%s max receive count = %q, want 3", seedOrders, got)
	}
	if got := orders.Attribute(AttrFIFO); got != "false" {
		t.Errorf("%s reports fifo=%q", seedOrders, got)
	}

	// A delayed message is held and is not available, which is a distinction
	// no single figure can carry.
	delayed := destinationNamed(destinations, seedDelayed)
	if delayed == nil {
		e2e.Missing(t, "%s is not in the listing; run `npm run e2e:sqs:seed`", seedDelayed)
	}
	// The seed holds them back by 15 minutes, which is the longest SQS allows.
	// A local run an hour later finds them visible, and that is the seed
	// having aged rather than the driver being wrong.
	if delayed.Attribute(AttrDelayed) == "0" {
		e2e.Missing(t, "%s no longer holds a delayed message; the seed's delay has run out, so re-run `npm run e2e:sqs:seed`", seedDelayed)
	}
	if delayed.Attribute(AttrVisible) != "0" || delayed.Attribute(AttrDelayed) != "5" {
		t.Errorf("%s visible=%q delayed=%q, want 0 and 5",
			seedDelayed, delayed.Attribute(AttrVisible), delayed.Attribute(AttrDelayed))
	}
	if delayed.Depth != 5 {
		t.Errorf("%s depth = %d; a delayed message is still held", seedDelayed, delayed.Depth)
	}

	fifo := destinationNamed(destinations, seedFIFO)
	if fifo == nil {
		e2e.Missing(t, "%s is not in the listing; run `npm run e2e:sqs:seed`", seedFIFO)
	}
	if fifo.Attribute(AttrFIFO) != "true" {
		t.Errorf("%s is a FIFO queue and the listing says fifo=%q", seedFIFO, fifo.Attribute(AttrFIFO))
	}

	// An empty queue is a row, not an absence.
	if destinationNamed(destinations, seedEmpty) == nil {
		t.Errorf("%s is missing from the listing; an empty queue is still a queue", seedEmpty)
	}
}

/*
 * SQS keeps no record of who reads a queue, so the subscriber count has to be
 * unknown rather than zero. Zero reads as "nothing is consuming this", which is
 * a claim the service cannot support - and on the queues board it is the
 * difference between an honest dash and a false alarm.
 */
func TestLiveListDestinationsReportsNoSubscribersOrPartitions(t *testing.T) {
	conn := liveConn(t)

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(destinations) == 0 {
		e2e.Missing(t, "the region holds no queues; run `npm run e2e:sqs:seed`")
	}
	for _, destination := range destinations {
		if destination.Subscribers != model.UnknownMetric {
			t.Errorf("%s reports %d subscribers, and SQS knows of none",
				destination.Ref.Name, destination.Subscribers)
		}
		if destination.Partitions != model.UnknownMetric {
			t.Errorf("%s reports %d partitions, and an SQS queue is not split",
				destination.Ref.Name, destination.Partitions)
		}
		if destination.RateIn != model.UnknownMetric || destination.RateOut != model.UnknownMetric {
			t.Errorf("%s reports a rate, and SQS publishes none", destination.Ref.Name)
		}
	}
}

// The prefix is the only filter SQS has, and it is applied by the service. A
// connection scoped to one team's queues must not see another team's.
func TestLiveQueuePrefixNarrowsTheListing(t *testing.T) {
	requireRegion(t)

	profile := liveProfile()
	profile.Options[OptionQueuePrefix] = "MQS-SEED-orders"
	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(destinations) == 0 {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:sqs:seed`", seedOrders)
	}
	for _, destination := range destinations {
		if !strings.HasPrefix(destination.Ref.Name, "MQS-SEED-orders") {
			t.Errorf("%s is outside the connection's prefix", destination.Ref.Name)
		}
	}
	if destinationNamed(destinations, seedDelayed) != nil {
		t.Errorf("%s is outside the prefix and still listed", seedDelayed)
	}
}

// The detail read is one request against one queue rather than a walk of the
// listing, and it has to answer the same figures.
func TestLiveDestinationDetailMatchesTheListing(t *testing.T) {
	conn := liveConn(t)

	listed, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	fromList := destinationNamed(listed, seedOrders)
	if fromList == nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:sqs:seed`", seedOrders)
	}

	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedOrders})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Ref.Name != fromList.Ref.Name || detail.Depth != fromList.Depth {
		t.Errorf("detail says %s/%d and the listing says %s/%d",
			detail.Ref.Name, detail.Depth, fromList.Ref.Name, fromList.Depth)
	}
	if detail.Attribute(AttrARN) == "" {
		t.Error("the detail read reports no ARN, which is how every cross-queue setting names a queue")
	}

	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: "MQS-TEST-absent"}); err == nil {
		t.Error("described a queue that does not exist")
	}
}

// testName builds a name no other run collides with. SQS allows letters,
// digits, hyphens and underscores, and 80 characters - and refuses a deleted
// queue's name for 60 seconds, so a rerun cannot reuse one.
func testName(t *testing.T, suffix string) string {
	t.Helper()
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, t.Name())
	if len(safe) > 40 {
		safe = safe[:40]
	}
	return "MQS-TEST-" + safe + "-" + strconv.FormatInt(time.Now().UnixNano()%1e9, 36) + suffix
}

// makeQueue creates a queue for one test and removes it afterwards.
func makeQueue(t *testing.T, conn *Conn, spec QueueSpec) string {
	t.Helper()
	if err := conn.CreateQueue(liveContext(t), spec); err != nil {
		t.Fatalf("create %s: %v", spec.Name, err)
	}
	t.Cleanup(func() {
		_ = conn.RemoveDestination(context.Background(), model.DestinationRef{Name: spec.Name})
	})
	url, err := conn.queueURL(liveContext(t), spec.Name)
	if err != nil {
		t.Fatalf("resolve %s: %v", spec.Name, err)
	}
	return url
}

// A queue is created with the settings the form collected, and reads back with
// them: an attribute silently dropped between the form and the service would
// otherwise look like a queue that took a default nobody chose.
func TestLiveCreateQueueAppliesEverySetting(t *testing.T) {
	conn := liveConn(t)
	name := testName(t, "")

	makeQueue(t, conn, QueueSpec{
		Name:                 name,
		VisibilityTimeoutSec: 45,
		DelaySec:             10,
		RetentionSec:         3600,
		MaxMessageBytes:      65536,
		ReceiveWaitSec:       5,
	})

	queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	for _, want := range []struct{ key, value string }{
		{AttrVisibilityTimeo, "45"},
		{AttrDelaySeconds, "10"},
		{AttrRetentionSec, "3600"},
		{AttrMaxMessageBytes, "65536"},
		{AttrReceiveWaitSec, "5"},
		{AttrFIFO, "false"},
	} {
		if got := queue.Attribute(want.key); got != want.value {
			t.Errorf("%s = %q, want %q", want.key, got, want.value)
		}
	}
}

/*
 * FIFO is decided by the name and by nothing else, so the two have to agree
 * before the request is sent. SQS's own refusal names the FifoQueue attribute,
 * which is a field no form here draws.
 */
func TestLiveCreateQueueHoldsFIFOToItsName(t *testing.T) {
	conn := liveConn(t)

	t.Run("a fifo queue whose name says so", func(t *testing.T) {
		name := testName(t, ".fifo")
		makeQueue(t, conn, QueueSpec{Name: name, FIFO: true, ContentBasedDeduplication: true})

		queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		if queue.Attribute(AttrFIFO) != "true" {
			t.Errorf("fifo = %q, want true", queue.Attribute(AttrFIFO))
		}
		if queue.Attribute(AttrContentDedup) != "true" {
			t.Errorf("contentBasedDeduplication = %q, want true", queue.Attribute(AttrContentDedup))
		}
	})

	t.Run("a fifo queue whose name does not", func(t *testing.T) {
		err := conn.CreateQueue(liveContext(t), QueueSpec{Name: testName(t, ""), FIFO: true})
		if err == nil {
			t.Fatal("created a FIFO queue with no .fifo suffix")
		}
		if !strings.Contains(err.Error(), ".fifo") {
			t.Errorf("error does not name the suffix: %v", err)
		}
	})

	t.Run("a standard queue whose name says fifo", func(t *testing.T) {
		err := conn.CreateQueue(liveContext(t), QueueSpec{Name: testName(t, ".fifo"), FIFO: false})
		if err == nil {
			t.Fatal("created a standard queue with a .fifo suffix")
		}
	})
}

// A redrive policy names the target by ARN and a person names it by name, so
// the driver resolves one into the other. The listing reads it back the other
// way, which is what puts the dead-letter column on the queues board.
func TestLiveCreateQueueResolvesTheDeadLetterQueueByName(t *testing.T) {
	conn := liveConn(t)
	dlq := testName(t, "-dlq")
	source := testName(t, "-src")

	makeQueue(t, conn, QueueSpec{Name: dlq})
	makeQueue(t, conn, QueueSpec{Name: source, DeadLetterQueue: dlq, MaxReceiveCount: 4})

	queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: source})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got := queue.Attribute(AttrDeadLetterQueue); got != dlq {
		t.Errorf("dead-letter queue = %q, want %q", got, dlq)
	}
	if got := queue.Attribute(AttrMaxReceiveCount); got != "4" {
		t.Errorf("max receive count = %q, want 4", got)
	}

	// A target that is not there has to say which queue it could not find,
	// rather than surfacing the service's InvalidParameterValue.
	err = conn.CreateQueue(liveContext(t), QueueSpec{
		Name: testName(t, "-orphan"), DeadLetterQueue: "MQS-TEST-no-such-dlq",
	})
	if err == nil {
		t.Fatal("created a queue pointing at a dead-letter queue that does not exist")
	}
	if !strings.Contains(err.Error(), "MQS-TEST-no-such-dlq") {
		t.Errorf("error does not name the missing queue: %v", err)
	}
}

/*
 * An edit writes only what the form sent. SQS replaces exactly the attributes
 * it is given, so a setting the form left alone has to survive - otherwise
 * changing a visibility timeout would silently reset a queue's retention to
 * the service default.
 */
func TestLiveUpdateQueueLeavesUnsentSettingsAlone(t *testing.T) {
	conn := liveConn(t)
	name := testName(t, "")
	makeQueue(t, conn, QueueSpec{Name: name, VisibilityTimeoutSec: 45, RetentionSec: 3600})

	if err := conn.UpdateQueue(liveContext(t), QueueSpec{Name: name, VisibilityTimeoutSec: 90}); err != nil {
		t.Fatalf("update: %v", err)
	}

	queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if got := queue.Attribute(AttrVisibilityTimeo); got != "90" {
		t.Errorf("visibility timeout = %q, want the edited 90", got)
	}
	if got := queue.Attribute(AttrRetentionSec); got != "3600" {
		t.Errorf("retention = %q, want the untouched 3600", got)
	}
}

// Whether a queue is FIFO is fixed at creation. SetQueueAttributes answers
// "Unknown Attribute FifoQueue", which names something the form never drew, so
// the driver refuses it with a message that says what to do instead.
func TestLiveUpdateQueueRefusesToChangeFIFO(t *testing.T) {
	conn := liveConn(t)
	name := testName(t, "")
	makeQueue(t, conn, QueueSpec{Name: name})

	err := conn.UpdateQueue(liveContext(t), QueueSpec{Name: name, FIFO: true})
	if err == nil {
		t.Fatal("turned an existing standard queue into a FIFO one")
	}
	if !strings.Contains(err.Error(), "create a new queue") {
		t.Errorf("error does not say what to do instead: %v", err)
	}
}

/*
 * Purging is asynchronous, so this waits for the queue to report empty rather
 * than asserting straight after the call. The wait is the assertion: a purge
 * that did nothing never reaches zero.
 */
func TestLivePurgeQueueEmptiesIt(t *testing.T) {
	conn := liveConn(t)
	name := testName(t, "")
	url := makeQueue(t, conn, QueueSpec{Name: name})
	sendRaw(t, conn, url, 5)

	if err := conn.PurgeQueue(liveContext(t), model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	waitForDepth(t, conn, name, 0)
}

// A delete removes the queue from the listing, and the cached URL with it: a
// call under the same name afterwards has to ask again rather than address a
// queue that has gone.
func TestLiveRemoveQueueTakesItOutOfTheListing(t *testing.T) {
	conn := liveConn(t)
	name := testName(t, "")
	makeQueue(t, conn, QueueSpec{Name: name})

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("remove: %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for {
		destinations, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if destinationNamed(destinations, name) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s is still listed 20s after it was deleted", name)
		}
		time.Sleep(time.Second)
	}

	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name}); err == nil {
		t.Error("described a queue that was deleted; the cached url outlived it")
	}
}

// sendRaw puts messages on a queue through the SDK rather than through this
// driver, which has no send until the publish capability lands.
func sendRaw(t *testing.T, conn *Conn, url string, count int) {
	t.Helper()
	for index := range count {
		_, err := conn.client.SendMessage(liveContext(t), &awssqs.SendMessageInput{
			QueueUrl:    aws.String(url),
			MessageBody: aws.String(fmt.Sprintf("body-%d", index+1)),
		})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
	}
}

// waitForDepth waits for a queue to report a depth, because every SQS figure
// is what its servers last agreed on rather than what is true this instant.
func waitForDepth(t *testing.T, conn *Conn, name string, want int64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last int64
	for {
		queue, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
		if err != nil {
			t.Fatalf("detail: %v", err)
		}
		last = queue.Depth
		if last == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s reports a depth of %d after 30s, want %d", name, last, want)
		}
		time.Sleep(time.Second)
	}
}
