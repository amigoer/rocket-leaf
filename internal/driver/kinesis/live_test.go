package kinesis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

// The live environment, as tests/e2e/kinesis/compose.yaml publishes it.
//
// LocalStack, reached through the endpoint override. That is not a testing
// shortcut around the real code path: a VPC interface endpoint is configured
// with the same field, so every test here exercises it.
//
// The port is not the SQS stack's. Two LocalStack containers run at once on
// one machine, which is what keeps the two AWS families independent.
const (
	liveEndpoint  = "http://127.0.0.1:4567"
	liveRegion    = "eu-west-1"
	liveAccessKey = "test"
	liveSecretKey = "test"
)

// The streams scripts/e2e-kinesis-seed.sh creates. Tests read these and never
// write to them; anything a test needs to change it creates for itself.
const (
	seedOrders   = "MQS-SEED-orders"
	seedSplit    = "MQS-SEED-split"
	seedEmpty    = "MQS-SEED-empty"
	seedOnDemand = "MQS-SEED-ondemand"
)

func requireRegion(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.Kinesis,
		Name:   "the kinesis environment",
		Start:  "npm run e2e:kinesis:up",
		Probe:  e2e.HTTPGet(liveEndpoint + "/_localstack/health"),
	})
}

// liveProfile is the environment as a user would configure it: a region, a
// credential, and no address anywhere on the form.
func liveProfile() model.ConnectionProfile {
	profile := model.ConnectionProfile{
		ID:         1,
		Name:       "kinesis e2e",
		Kind:       model.KindKinesis,
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
 * A profile whose Endpoints is empty opens, which only the hosted families can
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
func TestLiveOpenProvesTheCredentialReachesKinesis(t *testing.T) {
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
		if _, err := open(liveContext(t), profile); err == nil {
			t.Fatal("opened with no region to sign for")
		} else if !strings.Contains(err.Error(), "region") {
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

// The consumer calls take a stream's ARN and nothing else, so the name a user
// types has to reach one - and a name nothing answers for has to say so rather
// than failing later with an SDK error naming an operation nobody asked for.
func TestLiveStreamARNResolvesANameAndRefusesAnUnknownOne(t *testing.T) {
	conn := liveConn(t)

	arn, err := conn.streamARN(liveContext(t), seedOrders)
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedOrders, err)
	}
	if !strings.HasSuffix(arn, ":stream/"+seedOrders) {
		t.Errorf("stream arn %q does not end in the stream name", arn)
	}
	if streamNameOf(arn) != seedOrders {
		t.Errorf("streamNameOf(%q) = %q, want %q", arn, streamNameOf(arn), seedOrders)
	}

	if _, err := conn.streamARN(liveContext(t), "MQS-TEST-not-here"); err == nil {
		t.Fatal("resolved a stream that does not exist")
	} else if !strings.Contains(err.Error(), "MQS-TEST-not-here") {
		t.Errorf("error does not name the stream that is missing: %v", err)
	}
}

/*
 * The listing, against the seeded region.
 *
 * Two calls per board and neither is optional: ListStreams answers with names,
 * and every figure a row shows is on DescribeStreamSummary. What this asserts
 * is that the fold produces one row per stream with the figures the board
 * reads, rather than that the two calls happened.
 */
func TestLiveListDestinationsDescribesEveryStream(t *testing.T) {
	conn := liveConn(t)

	listed, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	byName := make(map[string]*model.Destination, len(listed))
	for _, destination := range listed {
		byName[destination.Ref.Name] = destination
	}
	for _, name := range []string{seedOrders, seedSplit, seedEmpty, seedOnDemand} {
		if byName[name] == nil {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed`", name)
		}
	}

	orders := byName[seedOrders]
	if orders.Partitions != 3 {
		t.Errorf("%s reports %d open shards, want the 3 the seed created",
			seedOrders, orders.Partitions)
	}
	if orders.Attribute(AttrRetentionHours) != "48" {
		t.Errorf("retention = %q, want the 48 hours the seed set",
			orders.Attribute(AttrRetentionHours))
	}
	if orders.Attribute(AttrStatus) != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", orders.Attribute(AttrStatus))
	}
	if orders.Attribute(AttrMode) != "PROVISIONED" {
		t.Errorf("mode = %q, want PROVISIONED", orders.Attribute(AttrMode))
	}
	if !strings.HasSuffix(orders.Attribute(AttrARN), ":stream/"+seedOrders) {
		t.Errorf("arn = %q, which does not name the stream", orders.Attribute(AttrARN))
	}

	// The on-demand stream is the one whose capacity nobody here chose, and
	// the assertion is that it is not a second kind of object: it lists like
	// any other, with a shard count the service picked.
	onDemand := byName[seedOnDemand]
	if onDemand.Attribute(AttrMode) != "ON_DEMAND" {
		t.Errorf("%s mode = %q, want ON_DEMAND", seedOnDemand, onDemand.Attribute(AttrMode))
	}
	if onDemand.Partitions <= 0 {
		t.Errorf("%s reports %d open shards; an on-demand stream still has some",
			seedOnDemand, onDemand.Partitions)
	}
}

/*
 * The three figures this family cannot report, asserted as absent.
 *
 * A zero and an unknown look identical on a board that renders both as a
 * number, and the difference is the whole reason UnknownMetric exists: a
 * stream holding nothing and a stream whose depth nobody can measure are not
 * the same thing. Kinesis measures none of the three.
 */
func TestLiveListDestinationsReportsNoDepthOrRates(t *testing.T) {
	conn := liveConn(t)

	listed, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	if len(listed) == 0 {
		e2e.Missing(t, "the region holds no streams; run `npm run e2e:kinesis:seed`")
	}
	for _, destination := range listed {
		if destination.Depth != model.UnknownMetric {
			t.Errorf("%s reports a depth of %d, and nothing in Kinesis counts stored records",
				destination.Ref.Name, destination.Depth)
		}
		if destination.RateIn != model.UnknownMetric || destination.RateOut != model.UnknownMetric {
			t.Errorf("%s reports rates (%d in, %d out), which live in CloudWatch",
				destination.Ref.Name, destination.RateIn, destination.RateOut)
		}
	}
}

// The prefix narrows the listing, and it is applied here rather than by the
// service - ListStreams has no filter of its own, unlike ListQueues.
func TestLiveListDestinationsHonoursTheStreamPrefix(t *testing.T) {
	requireRegion(t)

	profile := liveProfile()
	profile.Options[OptionStreamPrefix] = "MQS-SEED-o"
	conn, err := open(liveContext(t), profile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	listed, err := conn.ListDestinations(liveContext(t), model.DestinationFilter{})
	if err != nil {
		t.Fatalf("ListDestinations: %v", err)
	}
	if len(listed) == 0 {
		e2e.Missing(t, "nothing matched MQS-SEED-o; run `npm run e2e:kinesis:seed`")
	}
	for _, destination := range listed {
		if !strings.HasPrefix(destination.Ref.Name, "MQS-SEED-o") {
			t.Errorf("prefix MQS-SEED-o let %q through", destination.Ref.Name)
		}
	}
}

// Describing one stream is one call, not a walk of the listing: an account
// with a thousand streams must not answer for all of them to draw one row.
func TestLiveDestinationDetailReadsOneStream(t *testing.T) {
	conn := liveConn(t)

	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedSplit})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedSplit, err)
	}
	// Two open children after the seed's split, and the closed parent is not
	// among them - which is the whole reason the count is the open one.
	if detail.Partitions != 2 {
		t.Errorf("%s reports %d open shards, want the 2 the split left",
			seedSplit, detail.Partitions)
	}

	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: "MQS-TEST-absent"}); err == nil {
		t.Fatal("described a stream that does not exist")
	} else if !strings.Contains(err.Error(), "MQS-TEST-absent") {
		t.Errorf("error does not name the missing stream: %v", err)
	}
}
