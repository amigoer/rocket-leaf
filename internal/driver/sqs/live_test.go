package sqs

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

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
