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
