package kinesis

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awskinesis "github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"

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

// testStream creates a stream the test owns and removes it afterwards.
//
// Every live test that changes anything works on one of these rather than on a
// seeded stream: the seed is what the cross-check compares against, and a test
// that resized or emptied one would take the other half of verification with
// it.
func testStream(t *testing.T, conn *Conn, name string, spec StreamSpec) string {
	t.Helper()
	spec.Name = name
	if err := conn.CreateStream(liveContext(t), spec); err != nil {
		t.Fatalf("CreateStream %s: %v", name, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = conn.RemoveDestination(ctx, model.DestinationRef{Name: name})
	})
	if err := conn.awaitActive(liveContext(t), name); err != nil {
		t.Fatalf("%s never became active: %v", name, err)
	}
	return name
}

// putRaw writes records through the SDK rather than through this driver.
//
// A test that is asserting how a browse reads has to put the records there by
// some other means, or it would be checking the driver against itself.
func putRaw(t *testing.T, conn *Conn, stream, body string, count int) {
	t.Helper()
	records := make([]types.PutRecordsRequestEntry, 0, count)
	for index := 0; index < count; index++ {
		records = append(records, types.PutRecordsRequestEntry{
			Data:         []byte(body),
			PartitionKey: aws.String(fmt.Sprintf("%s-%d", body, index)),
		})
	}
	out, err := conn.client.PutRecords(liveContext(t), &awskinesis.PutRecordsInput{
		StreamName: aws.String(stream),
		Records:    records,
	})
	if err != nil {
		t.Fatalf("PutRecords on %s: %v", stream, err)
	}
	if failed := aws.ToInt32(out.FailedRecordCount); failed != 0 {
		t.Fatalf("%d of %d records were rejected by %s", failed, count, stream)
	}
}

/*
 * Create, describe, resize and delete, against the service rather than a mock.
 *
 * The resize is the part worth running live. It is not a field being written:
 * UpdateShardCount splits and merges, which leaves the stream UPDATING and
 * every subsequent call refused until it settles - so a driver that returned
 * as soon as the call was accepted would work in isolation and fail the moment
 * two settings changed at once.
 */
func TestLiveCreateResizeAndDeleteAStream(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-lifecycle", StreamSpec{Shards: 1, RetentionHours: 24})

	created, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if created.Partitions != 1 {
		t.Errorf("created with %d open shards, want 1", created.Partitions)
	}
	if created.Attribute(AttrMode) != "PROVISIONED" {
		t.Errorf("mode = %q, want PROVISIONED", created.Attribute(AttrMode))
	}

	if err := conn.UpdateStream(liveContext(t), StreamSpec{Name: name, Shards: 2}); err != nil {
		t.Fatalf("UpdateStream to 2 shards: %v", err)
	}
	resized, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("DestinationDetail after resize: %v", err)
	}
	if resized.Partitions != 2 {
		t.Errorf("after the resize %s reports %d open shards, want 2", name, resized.Partitions)
	}

	if err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: name}); err != nil {
		t.Fatalf("RemoveDestination: %v", err)
	}
	if _, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name}); err == nil {
		t.Error("the deleted stream still describes")
	}
}

// The retention is not on CreateStream at all: a new stream keeps 24 hours and
// is changed afterwards, by a call that is refused until it is ACTIVE. So a
// create asking for anything else is two operations, and this asserts the
// second one happened rather than being silently dropped.
func TestLiveCreateAppliesARetentionCreateStreamCannotCarry(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-retention", StreamSpec{Shards: 1, RetentionHours: 72})

	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if detail.Attribute(AttrRetentionHours) != "72" {
		t.Errorf("retention = %q, want the 72 hours the create asked for",
			detail.Attribute(AttrRetentionHours))
	}

	// Down as well as up: the service refuses a decrease sent to the increase
	// endpoint, and the driver picks the call by direction rather than by what
	// the caller thinks it is doing.
	if err := conn.UpdateStream(liveContext(t), StreamSpec{Name: name, RetentionHours: 24}); err != nil {
		t.Fatalf("UpdateStream down to 24 hours: %v", err)
	}
	lowered, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("DestinationDetail after the decrease: %v", err)
	}
	if lowered.Attribute(AttrRetentionHours) != "24" {
		t.Errorf("retention = %q after a decrease, want 24", lowered.Attribute(AttrRetentionHours))
	}
}

// An on-demand stream's capacity is the service's. A shard count sent beside
// the mode is refused by CreateStream naming an argument the form never drew,
// so the driver refuses it where the message can name the switch instead.
func TestLiveOnDemandStreamTakesNoShardCount(t *testing.T) {
	conn := liveConn(t)

	// Removed whatever the assertion finds, because a stream left behind by a
	// failed run is refused by name on the next one - which reports "already
	// exists" where the real problem was this call succeeding.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = conn.RemoveDestination(ctx, model.DestinationRef{Name: "MQS-TEST-ondemand-bad"})
	})
	err := conn.CreateStream(liveContext(t), StreamSpec{
		Name: "MQS-TEST-ondemand-bad", OnDemand: true, Shards: 4,
	})
	if err == nil {
		t.Fatal("accepted a shard count on an on-demand stream")
	}
	if !strings.Contains(err.Error(), "on-demand") {
		t.Errorf("error does not name the mode that refused it: %v", err)
	}

	name := testStream(t, conn, "MQS-TEST-ondemand", StreamSpec{OnDemand: true})
	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if detail.Attribute(AttrMode) != "ON_DEMAND" {
		t.Errorf("mode = %q, want ON_DEMAND", detail.Attribute(AttrMode))
	}
	// The service still gives it shards; what it does not give is a number
	// anybody here chose.
	if detail.Partitions <= 0 {
		t.Errorf("an on-demand stream reports %d open shards", detail.Partitions)
	}
	if err := conn.UpdateStream(liveContext(t), StreamSpec{
		Name: name, OnDemand: true, Shards: 8,
	}); err == nil {
		t.Error("resized an on-demand stream, whose capacity is not the operator's")
	}
}

// The retention window is the service's own, and it is checked here so the
// message names the field rather than arriving as InvalidArgumentException.
func TestLiveCreateRefusesARetentionOutsideTheWindow(t *testing.T) {
	conn := liveConn(t)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = conn.RemoveDestination(ctx, model.DestinationRef{Name: "MQS-TEST-retention-bad"})
	})
	for _, hours := range []int{1, 23, 9000} {
		err := conn.CreateStream(liveContext(t), StreamSpec{
			Name: "MQS-TEST-retention-bad", Shards: 1, RetentionHours: hours,
		})
		if err == nil {
			t.Errorf("accepted a retention of %d hours, and created a stream doing it", hours)
			continue
		}
		if !strings.Contains(err.Error(), "hours") {
			t.Errorf("retention %d: error does not name the unit: %v", hours, err)
		}
	}
}

// Deleting a stream that is already gone has to say which one, rather than
// letting the SDK report an operation the user never asked for by name.
func TestLiveDeleteNamesAStreamThatIsNotThere(t *testing.T) {
	conn := liveConn(t)

	err := conn.RemoveDestination(liveContext(t), model.DestinationRef{Name: "MQS-TEST-never-existed"})
	if err == nil {
		t.Fatal("deleted a stream that does not exist")
	}
	if !strings.Contains(err.Error(), "MQS-TEST-never-existed") {
		t.Errorf("error does not name the missing stream: %v", err)
	}
}

/*
 * The concept this family needed a port for, read off a stream that has one.
 *
 * MQS-SEED-split was created with one shard, written to, then split. What the
 * seed leaves is the shape a count cannot describe: three shards, of which one
 * is closed and two name it as their parent, and the closed one still holds
 * the records written before the split. A driver that listed only the open
 * shards would report two rows and lose the records on the third.
 */
func TestLiveListShardsKeepsTheClosedParentOfASplit(t *testing.T) {
	conn := liveConn(t)

	shards, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: seedSplit})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedSplit, err)
	}
	if len(shards) != 3 {
		t.Fatalf("%s reports %d shards, want the closed parent and its two children",
			seedSplit, len(shards))
	}

	var closed, children int
	byID := make(map[string]*model.Shard, len(shards))
	for _, shard := range shards {
		byID[shard.ID] = shard
		if shard.Closed {
			closed++
			if shard.EndSequence == "" {
				t.Errorf("%s is closed with no ending sequence number, which is what closes it",
					shard.ID)
			}
		}
		if shard.ParentID != "" {
			children++
			if byID[shard.ParentID] == nil {
				t.Errorf("%s names %s as its parent and the listing does not carry it",
					shard.ID, shard.ParentID)
			}
		}
		if shard.StartHashKey == "" || shard.EndHashKey == "" {
			t.Errorf("%s reports no hash key range, which is what decides its records", shard.ID)
		}
	}
	if closed != 1 {
		t.Errorf("%d closed shards, want the one the split left", closed)
	}
	if children != 2 {
		t.Errorf("%d shards name a parent, want the two the split created", children)
	}

	// The open shards between them cover the whole key space, and the closed
	// parent covers it a second time - which is exactly why a listing cannot
	// be read as a partition table.
	detail, err := conn.DestinationDetail(liveContext(t), model.DestinationRef{Name: seedSplit})
	if err != nil {
		t.Fatalf("DestinationDetail: %v", err)
	}
	if detail.Partitions != len(shards)-closed {
		t.Errorf("the stream reports %d open shards and the listing has %d that are not closed",
			detail.Partitions, len(shards)-closed)
	}
}

// A stream nobody has resized has no closed shards and no lineage, which is
// the state the split test cannot show: every shard is open, none names a
// parent, and the ranges partition the key space exactly once.
func TestLiveListShardsOnAnUntouchedStream(t *testing.T) {
	conn := liveConn(t)

	shards, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: seedOrders})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedOrders, err)
	}
	if len(shards) != 3 {
		t.Fatalf("%s reports %d shards, want the 3 the seed created", seedOrders, len(shards))
	}
	for _, shard := range shards {
		if shard.Closed {
			t.Errorf("%s is closed on a stream nothing has resized", shard.ID)
		}
		if shard.ParentID != "" || shard.AdjacentParentID != "" {
			t.Errorf("%s names a parent on a stream nothing has split", shard.ID)
		}
		if shard.StartSequence == "" {
			t.Errorf("%s reports no starting sequence number", shard.ID)
		}
	}

	if _, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: "MQS-TEST-no-shards"}); err == nil {
		t.Fatal("listed the shards of a stream that does not exist")
	} else if !strings.Contains(err.Error(), "MQS-TEST-no-shards") {
		t.Errorf("error does not name the missing stream: %v", err)
	}
}

/*
 * A merge, which is the half of the lineage a split cannot show.
 *
 * A merged shard has two parents rather than one, and the second is
 * AdjacentParentShardId - a field nothing else in this app has a counterpart
 * for. It is exercised here rather than in the seed because a merge needs two
 * adjacent open shards to consume, which the seeded split has and the seeded
 * stream must keep.
 */
func TestLiveListShardsRecordsBothParentsOfAMerge(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-merge", StreamSpec{Shards: 2})

	before, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("created with %d shards, want 2 to merge", len(before))
	}

	// Two shards merge into one, which the resize path spells as a target
	// count: the driver offers no shard-level split or merge, and this is what
	// the service does with the request.
	if err := conn.UpdateStream(liveContext(t), StreamSpec{Name: name, Shards: 1}); err != nil {
		t.Fatalf("UpdateStream down to 1 shard: %v", err)
	}

	after, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("ListShards after the merge: %v", err)
	}
	var merged *model.Shard
	for _, shard := range after {
		if shard.AdjacentParentID != "" {
			merged = shard
		}
	}
	if merged == nil {
		t.Fatalf("no shard names a second parent after a merge; got %d shards", len(after))
	}
	if merged.ParentID == "" {
		t.Error("the merged shard names an adjacent parent and no first parent")
	}
	if merged.ParentID == merged.AdjacentParentID {
		t.Errorf("both parents are %s; a merge consumes two shards", merged.ParentID)
	}
	if merged.Closed {
		t.Error("the shard a merge produced is closed, and it is the one taking writes")
	}
}

/*
 * The claim the caveat rests on, proved rather than assumed.
 *
 * Every other hosted family here warns that browsing takes the message away
 * from a consumer. This asserts the opposite for Kinesis, twice over: two
 * browses of the same stream return the same records, and a third read of one
 * shard from the horizon returns them again. If GetRecords ever consumed,
 * hid, or marked anything, one of the three would come back short.
 */
func TestLiveBrowsingTakesNothingAway(t *testing.T) {
	conn := liveConn(t)

	first, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(first) == 0 {
		e2e.Missing(t, "%s holds no records; run `npm run e2e:kinesis:seed`", seedOrders)
	}

	second, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("second QueryMessages: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("the second browse returned %d records where the first returned %d; "+
			"reading a kinesis stream must take nothing", len(second), len(first))
	}
	for index := range first {
		if first[index].MessageID != second[index].MessageID {
			t.Errorf("record %d is %s on the first browse and %s on the second",
				index, first[index].MessageID, second[index].MessageID)
		}
	}

	// A third read, of one shard, from the beginning: nothing about the first
	// two moved a cursor the service keeps, because it keeps none.
	shard := first[0].Properties[PropShardID]
	again, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      seedOrders,
		MaxResults: 100,
		Filters:    map[string]string{FilterShardID: shard},
	})
	if err != nil {
		t.Fatalf("browsing %s: %v", shard, err)
	}
	if len(again) == 0 {
		t.Errorf("%s came back empty after two whole-stream browses", shard)
	}
}

// A browse reads every shard rather than the first one, which is the whole
// reason the read is a fan-out: a stream's records are spread by partition key
// hash, so a page that read one shard would show a third of an even stream.
func TestLiveBrowsingReadsEveryShard(t *testing.T) {
	conn := liveConn(t)

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 200,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) == 0 {
		e2e.Missing(t, "%s holds no records; run `npm run e2e:kinesis:seed`", seedOrders)
	}

	shards := map[string]int{}
	for _, message := range messages {
		shard := message.Properties[PropShardID]
		if shard == "" {
			t.Errorf("%s carries no shard id, and its sequence number means nothing without one",
				message.MessageID)
		}
		shards[shard]++
		if !strings.HasPrefix(message.MessageID, shard+":") {
			t.Errorf("message id %q does not start with its shard", message.MessageID)
		}
		if message.Keys == "" {
			t.Errorf("%s carries no partition key, which is what placed it", message.MessageID)
		}
	}
	if len(shards) < 2 {
		t.Errorf("every record came from %d shard(s); the seed spreads them over 3", len(shards))
	}
	// Newest first, which is what every other browse in this app shows.
	for index := 1; index < len(messages); index++ {
		if messages[index-1].StoreTimestamp < messages[index].StoreTimestamp {
			t.Errorf("record %d arrived before record %d and is listed after it", index-1, index)
			break
		}
	}
}

/*
 * A browse of a split stream has to reach the closed parent.
 *
 * MQS-SEED-split holds eight records written before the split and six after.
 * The eight are on a shard that takes no more writes, and a browse that read
 * only the open shards would show the six and silently lose the rest - which
 * is the same mistake as hiding a closed shard on the shards page, met here.
 */
func TestLiveBrowsingReachesRecordsOnAClosedShard(t *testing.T) {
	conn := liveConn(t)

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedSplit, MaxResults: 200,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) == 0 {
		e2e.Missing(t, "%s holds no records; run `npm run e2e:kinesis:seed`", seedSplit)
	}

	shards, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: seedSplit})
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}
	closed := map[string]bool{}
	for _, shard := range shards {
		if shard.Closed {
			closed[shard.ID] = true
		}
	}

	var fromClosed int
	for _, message := range messages {
		if closed[message.Properties[PropShardID]] {
			fromClosed++
		}
	}
	if fromClosed == 0 {
		t.Errorf("none of the %d records came from a closed shard, and the seed wrote 8 "+
			"before the split", len(messages))
	}
	if len(messages) != 14 {
		t.Errorf("the browse found %d records; the seed wrote 8 before the split and 6 after",
			len(messages))
	}
}

// The pair that addresses a record, exercised both ways: the id a browse
// produces fetches exactly that record back, and half of it fetches nothing -
// a sequence number means nothing without the shard that holds it.
func TestLiveMessageByIDNeedsBothHalvesOfTheID(t *testing.T) {
	conn := liveConn(t)

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: seedOrders, MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	if len(messages) == 0 {
		e2e.Missing(t, "%s holds no records; run `npm run e2e:kinesis:seed`", seedOrders)
	}
	wanted := messages[0]

	found, err := conn.MessageByID(liveContext(t), seedOrders, wanted.MessageID)
	if err != nil {
		t.Fatalf("MessageByID(%s): %v", wanted.MessageID, err)
	}
	if found.MessageID != wanted.MessageID {
		t.Errorf("fetched %s, want %s", found.MessageID, wanted.MessageID)
	}
	if found.Body != wanted.Body {
		t.Errorf("body = %q, want %q", found.Body, wanted.Body)
	}

	bare := wanted.Properties[PropSequenceNumber]
	if _, err := conn.MessageByID(liveContext(t), seedOrders, bare); err == nil {
		t.Error("fetched a record by sequence number alone, which addresses nothing")
	} else if !strings.Contains(err.Error(), "shard") {
		t.Errorf("error does not say the shard is missing: %v", err)
	}

	// The other half of the same fact: a real sequence number offered against
	// the wrong shard is refused by the service rather than silently reading
	// from the start of it.
	other := ""
	for _, message := range messages {
		if message.Properties[PropShardID] != wanted.Properties[PropShardID] {
			other = message.Properties[PropShardID]
			break
		}
	}
	if other == "" {
		e2e.Missing(t, "every seeded record landed on one shard; run `npm run e2e:kinesis:seed`")
	}
	if _, err := conn.MessageByID(liveContext(t), seedOrders, other+":"+bare); err == nil {
		t.Errorf("a sequence number from %s was accepted against %s",
			wanted.Properties[PropShardID], other)
	}
}

// A start time moves the iterator rather than filtering afterwards, which is
// what keeps a narrow search from reading the whole retention period. An end
// time has to be applied here, because the service has no server-side
// selection of any kind.
func TestLiveBrowsingHonoursATimeWindow(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-window", StreamSpec{Shards: 1})

	putRaw(t, conn, name, "early", 3)
	// A whole second, because the arrival timestamp has millisecond
	// resolution and AT_TIMESTAMP is inclusive.
	time.Sleep(time.Second)
	boundary := time.Now().UTC()
	time.Sleep(time.Second)
	putRaw(t, conn, name, "late", 2)

	after, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: name, MaxResults: 50, StartTime: boundary.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("QueryMessages from the boundary: %v", err)
	}
	if len(after) != 2 {
		t.Errorf("a browse from the boundary found %d records, want the 2 written after it",
			len(after))
	}
	for _, message := range after {
		if message.Body != "late" {
			t.Errorf("a browse from the boundary returned %q", message.Body)
		}
	}

	before, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: name, MaxResults: 50, EndTime: boundary.UnixMilli(),
	})
	if err != nil {
		t.Fatalf("QueryMessages up to the boundary: %v", err)
	}
	if len(before) != 3 {
		t.Errorf("a browse up to the boundary found %d records, want the 3 written before it",
			len(before))
	}
}

/*
 * What LocalStack cannot exercise, recorded rather than left as a silent hole.
 *
 * The caveat on browsing is about the per-shard read allowance: five
 * GetRecords a second and two megabytes a second, shared with every classic
 * consumer on that shard. LocalStack reimplements the API rather than running
 * Amazon's, and it enforces neither - twenty calls in a quarter of a second
 * all succeed here, where AWS would answer some of them with
 * ProvisionedThroughputExceededException.
 *
 * So this asserts the half that can be asserted: the driver's own budget, five
 * GetRecords per shard per browse, which is what keeps a page from spending
 * more than one second of an application's read capacity. If the emulator ever
 * does enforce the quota, this test is where that shows up - the loop below
 * would start failing and the driver's handling of the exception would need a
 * test of its own.
 */
func TestLiveLocalStackDoesNotEnforceTheReadQuota(t *testing.T) {
	conn := liveConn(t)

	if browseCallsPerShard != 5 {
		t.Errorf("the per-shard call budget is %d; the service allows five GetRecords a second",
			browseCallsPerShard)
	}

	// Ten browses of one shard back to back, which is well past the allowance
	// a real shard has. Every one of them succeeds against the emulator.
	shard, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: seedOrders})
	if err != nil {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", seedOrders, err)
	}
	for attempt := 0; attempt < 10; attempt++ {
		_, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
			Topic:      seedOrders,
			MaxResults: 50,
			Filters:    map[string]string{FilterShardID: shard[0].ID},
		})
		if err == nil {
			continue
		}
		t.Fatalf("browse %d failed: %v\n"+
			"if this is a throughput error, the emulator now enforces the read quota "+
			"and the driver's handling of it needs a test rather than this note", attempt, err)
	}
}

/*
 * Sending, and the two fields that decide where a record lands.
 *
 * A partition key is not decoration: the service hashes it into the key space
 * and the shard whose range covers the hash takes the record. So a send
 * reports which shard took it, and the pair it returns is the same handle a
 * browse produces.
 */
func TestLivePublishReportsWhereTheRecordLanded(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-publish", StreamSpec{Shards: 2})

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Stream: name, Body: "one record", PartitionKey: "orders-1",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Sent != 1 || result.Failed != 0 {
		t.Errorf("sent %d and %d were refused, want 1 and 0", result.Sent, result.Failed)
	}
	if result.ShardID == "" || result.SequenceNumber == "" {
		t.Fatalf("the send reported shard %q and sequence %q; neither addresses a record alone",
			result.ShardID, result.SequenceNumber)
	}

	// The pair the send reported is the id the browse produces, which is what
	// makes "look at what I just wrote" a single step.
	found, err := conn.MessageByID(liveContext(t), name, result.ShardID+":"+result.SequenceNumber)
	if err != nil {
		t.Fatalf("MessageByID on what was just sent: %v", err)
	}
	if found.Body != "one record" {
		t.Errorf("body = %q, want %q", found.Body, "one record")
	}
	if found.Keys != "orders-1" {
		t.Errorf("partition key = %q, want orders-1", found.Keys)
	}
}

// A repeated body has to spread. Sending every copy under one partition key
// would load one shard and leave the rest idle, which is a fair thing to ask
// for explicitly and a bad default - the browse afterwards would look as
// though the stream were not spreading records at all.
func TestLivePublishSpreadsARepeatedBodyAcrossShards(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-spread", StreamSpec{Shards: 3})

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Stream: name, Body: "repeated", PartitionKey: "batch", Count: 30,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.Sent != 30 {
		t.Fatalf("sent %d of 30, %d refused", result.Sent, result.Failed)
	}

	messages, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic: name, MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("QueryMessages: %v", err)
	}
	shards := map[string]int{}
	keys := map[string]bool{}
	for _, message := range messages {
		shards[message.Properties[PropShardID]]++
		keys[message.Keys] = true
	}
	if len(shards) < 2 {
		t.Errorf("30 records landed on %d shard(s) of 3", len(shards))
	}
	if len(keys) != 30 {
		t.Errorf("%d distinct partition keys for 30 records; each copy needs its own or "+
			"they all land together", len(keys))
	}
}

/*
 * An explicit hash key aims a record at a shard by name, and it is the reason
 * the shards page shows each shard's range.
 *
 * It also turns off the per-copy partition key: the point of setting one is
 * that every record goes to the same place, and varying the key underneath it
 * would be doing nothing while looking like it did something.
 */
func TestLivePublishAimsAtAShardWithAnExplicitHashKey(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-aim", StreamSpec{Shards: 3})

	shards, err := conn.ListShards(liveContext(t), model.DestinationRef{Name: name})
	if err != nil {
		t.Fatalf("ListShards: %v", err)
	}
	if len(shards) != 3 {
		t.Fatalf("created with %d shards, want 3", len(shards))
	}
	wanted := shards[2]

	result, err := conn.Publish(liveContext(t), PublishRequest{
		Stream:          name,
		Body:            "aimed",
		PartitionKey:    "ignored-by-the-hash-key",
		ExplicitHashKey: wanted.StartHashKey,
		Count:           5,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if result.ShardID != wanted.ID {
		t.Errorf("the aimed send landed on %s, want %s", result.ShardID, wanted.ID)
	}

	onTarget, err := conn.QueryMessages(liveContext(t), model.MessageQueryParams{
		Topic:      name,
		MaxResults: 50,
		Filters:    map[string]string{FilterShardID: wanted.ID},
	})
	if err != nil {
		t.Fatalf("browsing %s: %v", wanted.ID, err)
	}
	if len(onTarget) != 5 {
		t.Errorf("%s holds %d of the 5 aimed records", wanted.ID, len(onTarget))
	}
	for _, message := range onTarget {
		if message.Keys != "ignored-by-the-hash-key" {
			t.Errorf("partition key = %q; an aimed send keeps the key it was given",
				message.Keys)
		}
	}

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Stream: name, Body: "bad", PartitionKey: "k", ExplicitHashKey: "not-a-number",
	}); err == nil {
		t.Error("accepted an explicit hash key that is not a number")
	}
}

/*
 * The canonical port, and the two arguments it has that Kinesis does not.
 *
 * Both are refused rather than ignored. A tag would be silently discarded and
 * the send reported as having carried it; a delay would report a scheduled
 * send that happened at once, which is the mistake ActiveMQ's driver was
 * written to avoid making.
 */
func TestLiveSendMessageRefusesWhatKinesisCannotCarry(t *testing.T) {
	conn := liveConn(t)
	name := testStream(t, conn, "MQS-TEST-send", StreamSpec{Shards: 1})

	id, err := conn.SendMessage(liveContext(t), name, "", "orders-1", "through the port", 0)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.Contains(id, idSeparator) {
		t.Errorf("SendMessage returned %q, which is not a shard and a sequence number", id)
	}
	if _, err := conn.MessageByID(liveContext(t), name, id); err != nil {
		t.Errorf("the id SendMessage returned does not address the record: %v", err)
	}

	if _, err := conn.SendMessage(liveContext(t), name, "urgent", "k", "body", 0); err == nil {
		t.Error("accepted a tag, which a kinesis record has nowhere to put")
	}
	if _, err := conn.SendMessage(liveContext(t), name, "", "k", "body", 3); err == nil {
		t.Error("accepted a delay, and nothing in kinesis holds a record back")
	}
}

// A send with no partition key is refused here rather than by the service,
// whose own message names Data and PartitionKey together without saying which
// was missing.
func TestLivePublishRefusesASendWithNoPartitionKey(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.Publish(liveContext(t), PublishRequest{Stream: seedOrders, Body: "body"})
	if err == nil {
		t.Fatal("accepted a record with no partition key")
	}
	if !strings.Contains(err.Error(), "partition key") {
		t.Errorf("error does not name the missing field: %v", err)
	}

	if _, err := conn.Publish(liveContext(t), PublishRequest{
		Stream: seedOrders, PartitionKey: "k",
	}); err == nil {
		t.Error("accepted an empty record")
	}
}

/*
 * Registered consumers, which are the only readers a stream knows about.
 *
 * The seed registers two on MQS-SEED-orders. What this asserts beyond their
 * being listed is the shape: a consumer is addressed by its stream and its
 * name together, and every figure a consumers page would want is absent rather
 * than zero - there is no position anywhere in the service to compute one
 * from.
 */
func TestLiveListSubscriptionsFindsRegisteredConsumers(t *testing.T) {
	conn := liveConn(t)

	subscriptions, err := conn.ListSubscriptions(liveContext(t))
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	found := map[string]*model.Subscription{}
	for _, subscription := range subscriptions {
		if subscription.Ref.Namespace == seedOrders {
			found[subscription.Ref.Name] = subscription
		}
	}
	if len(found) < 2 {
		e2e.Missing(t, "%s has %d registered consumers; run `npm run e2e:kinesis:seed`",
			seedOrders, len(found))
	}

	for name, subscription := range found {
		if subscription.Backlog != model.UnknownMetric {
			t.Errorf("%s reports a backlog of %d, and no call in the API returns one",
				name, subscription.Backlog)
		}
		if subscription.Members != model.UnknownMetric {
			t.Errorf("%s reports %d members, and the stream keeps no record of who is attached",
				name, subscription.Members)
		}
		if subscription.RateOut != model.UnknownMetric {
			t.Errorf("%s reports a consume rate, which lives in CloudWatch", name)
		}
		if subscription.Destinations != 1 {
			t.Errorf("%s reads %d streams; a consumer is registered on exactly one",
				name, subscription.Destinations)
		}
		if subscription.Attribute(AttrConsumerARN) == "" {
			t.Errorf("%s carries no consumer ARN, which is what an application subscribes with",
				name)
		}
	}
}

// Register, describe and deregister, against the service. The name is unique
// within its stream and nowhere else, which is why every call takes both.
func TestLiveRegisterAndDeregisterAConsumer(t *testing.T) {
	conn := liveConn(t)
	stream := testStream(t, conn, "MQS-TEST-consumers", StreamSpec{Shards: 1})
	ref := model.SubscriptionRef{Namespace: stream, Name: "MQS-TEST-reader"}

	if err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{Ref: ref}); err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	// Registration is asynchronous: the consumer is CREATING before it is
	// ACTIVE, which is what the warning status is for.
	var detail *model.Subscription
	deadline := time.Now().Add(30 * time.Second)
	for {
		found, err := conn.SubscriptionDetail(liveContext(t), ref)
		if err != nil {
			t.Fatalf("SubscriptionDetail: %v", err)
		}
		detail = found
		if detail.Status == model.SubscriptionOnline || time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if detail.Status != model.SubscriptionOnline {
		t.Errorf("status = %q after registering, want online", detail.Status)
	}
	if !strings.Contains(detail.Attribute(AttrConsumerARN), "/consumer/") {
		t.Errorf("consumer arn = %q, which does not name a consumer",
			detail.Attribute(AttrConsumerARN))
	}

	// A second registration under the same name is refused, and the message
	// has to name the stream as well - the name means nothing without it.
	err := conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{Ref: ref})
	if err == nil {
		t.Error("registered the same consumer name twice on one stream")
	} else if !strings.Contains(err.Error(), stream) {
		t.Errorf("error does not name the stream: %v", err)
	}

	if err := conn.RemoveSubscription(liveContext(t), ref); err != nil {
		t.Fatalf("RemoveSubscription: %v", err)
	}
	// Deregistration is asynchronous too, so a describe straight afterwards
	// may still answer - what matters is that the listing loses it.
	gone := false
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := conn.SubscriptionDetail(liveContext(t), ref); err != nil {
			gone = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !gone {
		t.Error("the deregistered consumer still describes")
	}
}

// A consumer name addresses nothing on its own: it is unique within its stream
// and every call takes the stream's ARN, so a ref with no namespace has to be
// refused where the message can say why.
func TestLiveConsumerRefNeedsItsStream(t *testing.T) {
	conn := liveConn(t)

	_, err := conn.SubscriptionDetail(liveContext(t), model.SubscriptionRef{Name: "reader"})
	if err == nil {
		t.Fatal("described a consumer without naming its stream")
	}
	if !strings.Contains(err.Error(), "stream") {
		t.Errorf("error does not say the stream is missing: %v", err)
	}

	err = conn.CreateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: seedOrders},
	})
	if err == nil {
		t.Error("registered a consumer with no name")
	}
}

/*
 * A registered consumer has nothing to update, and the update path says so
 * rather than accepting the call and doing nothing.
 *
 * Its name, ARN, status and creation time are all the service's, and every
 * setting a reader might want - retention, capacity, encryption - belongs to
 * the stream. No capability is declared for it, so nothing in the UI reaches
 * this; the error is what a future caller would find.
 */
func TestLiveConsumerUpdateIsRefusedRatherThanIgnored(t *testing.T) {
	conn := liveConn(t)

	err := conn.UpdateSubscription(liveContext(t), model.SubscriptionSpec{
		Ref: model.SubscriptionRef{Namespace: seedOrders, Name: "MQS-SEED-analytics"},
	})
	if err == nil {
		t.Fatal("accepted an update to a consumer that has nothing to change")
	}
}
