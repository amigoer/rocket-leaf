package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every Kinesis board, compared against the raw API.
 *
 * Almost every figure this family shows is something the driver assembled. The
 * streams board is a listing plus a describe per stream folded into one row;
 * the shards board is a paged listing whose closed flag is derived from the
 * presence of a field rather than read from one; a record's id is two values
 * joined, and its body is bytes turned into a string; and the consumers board
 * is one call per stream merged into a single list. Every one of those can be
 * subtly wrong and stay plausible, and the driver testing itself would produce
 * the same wrong answer twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * plain net/http against the same endpoint, its own SigV4, its own JSON. The
 * AWS SDK is deliberately not used here - the driver is a layer over it, and
 * using it on both sides would compare the driver against itself.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same region and create and delete streams of
 * their own while these are running.
 */

// rawKinesis is a minimal Kinesis client: one JSON endpoint, SigV4, and
// nothing else.
type rawKinesis struct {
	endpoint string
	region   string
	access   string
	secret   string
	client   *http.Client
}

func newRawKinesis() *rawKinesis {
	return &rawKinesis{
		endpoint: liveKinesisEndpoint,
		region:   liveKinesisRegion,
		access:   liveKinesisAccessKey,
		secret:   liveKinesisSecretKey,
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

/*
 * call signs and sends one request the way every AWS client does.
 *
 * Written out rather than imported, which is the whole point of this file: a
 * signature the driver's own SDK also produced would prove nothing about the
 * driver, and the scheme is a chain of HMACs over a canonical request.
 */
func (r *rawKinesis) call(t *testing.T, target string, payload map[string]any) map[string]any {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode %s: %v", target, err)
	}
	parsed, err := url.Parse(r.endpoint)
	if err != nil {
		t.Fatalf("parse the endpoint: %v", err)
	}

	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	datestamp := now.Format("20060102")
	hashed := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(hashed[:])

	headers := map[string]string{
		"content-type": "application/x-amz-json-1.1",
		"host":         parsed.Host,
		"x-amz-date":   stamp,
		"x-amz-target": "Kinesis_20131202." + target,
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, name := range names {
		canonicalHeaders.WriteString(name + ":" + headers[name] + "\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonical := strings.Join([]string{
		"POST", "/", "", canonicalHeaders.String(), signedHeaders, payloadHash,
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonical))
	scope := datestamp + "/" + r.region + "/kinesis/aws4_request"
	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", stamp, scope, hex.EncodeToString(canonicalHash[:]),
	}, "\n")

	sign := func(key []byte, message string) []byte {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(message))
		return mac.Sum(nil)
	}
	key := sign([]byte("AWS4"+r.secret), datestamp)
	key = sign(key, r.region)
	key = sign(key, "kinesis")
	key = sign(key, "aws4_request")
	signature := hex.EncodeToString(sign(key, toSign))

	request, err := http.NewRequest(http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build %s: %v", target, err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		r.access, scope, signedHeaders, signature))

	response, err := r.client.Do(request)
	if err != nil {
		t.Fatalf("%s: %v", target, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: %s: %s", target, response.Status, raw)
	}
	if len(raw) == 0 {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode %s: %v", target, err)
	}
	return decoded
}

func (r *rawKinesis) summary(t *testing.T, stream string) map[string]any {
	t.Helper()
	out := r.call(t, "DescribeStreamSummary", map[string]any{"StreamName": stream})
	summary, ok := out["StreamDescriptionSummary"].(map[string]any)
	if !ok {
		t.Fatalf("DescribeStreamSummary(%s) answered %v", stream, out)
	}
	return summary
}

func (r *rawKinesis) shards(t *testing.T, stream string) []map[string]any {
	t.Helper()
	var found []map[string]any
	payload := map[string]any{"StreamName": stream}
	for {
		out := r.call(t, "ListShards", payload)
		listed, _ := out["Shards"].([]any)
		for _, entry := range listed {
			if shard, ok := entry.(map[string]any); ok {
				found = append(found, shard)
			}
		}
		token, _ := out["NextToken"].(string)
		if token == "" {
			return found
		}
		payload = map[string]any{"NextToken": token}
	}
}

func number(t *testing.T, value any, what string) int {
	t.Helper()
	asFloat, ok := value.(float64)
	if !ok {
		t.Fatalf("%s is %v, not a number", what, value)
	}
	return int(asFloat)
}

/*
 * The streams board, row by row.
 *
 * The row is not a listing entry: ListStreams answers with names, and every
 * figure on it comes from a second call the driver makes and folds. So this
 * compares the folded row against the same two calls made independently -
 * which is what would catch a describe attached to the wrong name, a count
 * read off the wrong field, or a mode defaulted rather than read.
 */
func TestLiveKinesisCrossCheckStreamsBoard(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis crosscheck streams"))
	raw := newRawKinesis()

	listed, err := stack.destinations.List(kinesisContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byName := make(map[string]*model.Destination, len(listed))
	for _, entry := range listed {
		byName[entry.Ref.Name] = entry
	}

	for _, name := range []string{
		liveKinesisOrders, liveKinesisSplit, liveKinesisEmpty, liveKinesisOnDemand,
	} {
		row := byName[name]
		if row == nil {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed`", name)
		}
		summary := raw.summary(t, name)

		if want := number(t, summary["OpenShardCount"], name+" OpenShardCount"); row.Partitions != want {
			t.Errorf("%s: the board shows %d open shards, the API says %d",
				name, row.Partitions, want)
		}
		if want := number(t, summary["ConsumerCount"], name+" ConsumerCount"); row.Subscribers != want {
			t.Errorf("%s: the board shows %d consumers, the API says %d",
				name, row.Subscribers, want)
		}
		if want := number(t, summary["RetentionPeriodHours"], name+" retention"); row.Attribute(kinesisdriver.AttrRetentionHours) != fmt.Sprint(want) {
			t.Errorf("%s: the board shows %q hours of retention, the API says %d",
				name, row.Attribute(kinesisdriver.AttrRetentionHours), want)
		}
		if want, _ := summary["StreamARN"].(string); row.Attribute(kinesisdriver.AttrARN) != want {
			t.Errorf("%s: the board shows arn %q, the API says %q",
				name, row.Attribute(kinesisdriver.AttrARN), want)
		}
		if want, _ := summary["StreamStatus"].(string); row.Attribute(kinesisdriver.AttrStatus) != want {
			t.Errorf("%s: the board shows status %q, the API says %q",
				name, row.Attribute(kinesisdriver.AttrStatus), want)
		}
		mode, _ := summary["StreamModeDetails"].(map[string]any)
		if want, _ := mode["StreamMode"].(string); row.Attribute(kinesisdriver.AttrMode) != want {
			t.Errorf("%s: the board shows mode %q, the API says %q",
				name, row.Attribute(kinesisdriver.AttrMode), want)
		}

		// The three the board must never invent, whatever the API says.
		if row.Depth != model.UnknownMetric {
			t.Errorf("%s: the board shows a depth of %d, and no field in the API carries one",
				name, row.Depth)
		}
		if row.RateIn != model.UnknownMetric || row.RateOut != model.UnknownMetric {
			t.Errorf("%s: the board shows rates, and they are CloudWatch's", name)
		}
	}
}

/*
 * The shards board, which is the one page in this app no other family has.
 *
 * Two things are compared rather than one. The rows have to match ListShards
 * exactly - id, both parents and both ends of the hash range - and the closed
 * flag has to match the presence of an ending sequence number, because that
 * is derived rather than read: the API has no status field on a shard, and a
 * driver that guessed from the parent instead would be right on a split and
 * wrong on the shard that was merged into.
 */
func TestLiveKinesisCrossCheckShardsBoard(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis crosscheck shards"))
	raw := newRawKinesis()

	for _, name := range []string{liveKinesisSplit, liveKinesisOrders} {
		shown, err := stack.kinesis.Shards(kinesisContext(t), connID, name)
		if err != nil {
			e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed` (%v)", name, err)
		}
		actual := raw.shards(t, name)
		if len(shown) != len(actual) {
			t.Fatalf("%s: the board shows %d shards, the API lists %d",
				name, len(shown), len(actual))
		}

		byID := make(map[string]map[string]any, len(actual))
		for _, shard := range actual {
			id, _ := shard["ShardId"].(string)
			byID[id] = shard
		}
		for _, row := range shown {
			shard := byID[row.ID]
			if shard == nil {
				t.Errorf("%s: the board shows %s and the API does not list it", name, row.ID)
				continue
			}
			parent, _ := shard["ParentShardId"].(string)
			if row.ParentID != parent {
				t.Errorf("%s/%s: parent is %q on the board and %q in the API",
					name, row.ID, row.ParentID, parent)
			}
			adjacent, _ := shard["AdjacentParentShardId"].(string)
			if row.AdjacentParentID != adjacent {
				t.Errorf("%s/%s: adjacent parent is %q on the board and %q in the API",
					name, row.ID, row.AdjacentParentID, adjacent)
			}

			hashRange, _ := shard["HashKeyRange"].(map[string]any)
			start, _ := hashRange["StartingHashKey"].(string)
			end, _ := hashRange["EndingHashKey"].(string)
			if row.StartHashKey != start || row.EndHashKey != end {
				t.Errorf("%s/%s: the board shows the range %s-%s, the API says %s-%s",
					name, row.ID, row.StartHashKey, row.EndHashKey, start, end)
			}

			sequences, _ := shard["SequenceNumberRange"].(map[string]any)
			startSeq, _ := sequences["StartingSequenceNumber"].(string)
			endSeq, _ := sequences["EndingSequenceNumber"].(string)
			if row.StartSequence != startSeq {
				t.Errorf("%s/%s: starting sequence is %q on the board and %q in the API",
					name, row.ID, row.StartSequence, startSeq)
			}
			if row.EndSequence != endSeq {
				t.Errorf("%s/%s: ending sequence is %q on the board and %q in the API",
					name, row.ID, row.EndSequence, endSeq)
			}
			// The derived field, checked against what it is derived from.
			if row.Closed != (endSeq != "") {
				t.Errorf("%s/%s: the board calls it closed=%v, and its ending sequence is %q",
					name, row.ID, row.Closed, endSeq)
			}
		}
	}

	// And the two halves against each other: the open shard count on the
	// streams board is what the shards page has left after the closed ones.
	detail, err := stack.destinations.Detail(
		kinesisContext(t), connID, model.DestinationRef{Name: liveKinesisSplit})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	shown, err := stack.kinesis.Shards(kinesisContext(t), connID, liveKinesisSplit)
	if err != nil {
		t.Fatalf("Shards: %v", err)
	}
	open := 0
	for _, row := range shown {
		if !row.Closed {
			open++
		}
	}
	if detail.Partitions != open {
		t.Errorf("the streams board says %d open shards and the shards board lists %d that are not closed",
			detail.Partitions, open)
	}
}

/*
 * The records board, against the same shard read independently.
 *
 * Two things the driver does to a record can be wrong and stay plausible: the
 * body is bytes turned into a string, and the id is two values joined. Both
 * are compared here against a raw GetRecords on the same shard, whose payload
 * arrives base64 the way the wire carries it.
 */
func TestLiveKinesisCrossCheckRecordsBoard(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis crosscheck records"))
	raw := newRawKinesis()

	shards := raw.shards(t, liveKinesisOrders)
	if len(shards) == 0 {
		e2e.Missing(t, "%s is not seeded; run `npm run e2e:kinesis:seed`", liveKinesisOrders)
	}
	shardID, _ := shards[0]["ShardId"].(string)

	iterator := raw.call(t, "GetShardIterator", map[string]any{
		"StreamName":        liveKinesisOrders,
		"ShardId":           shardID,
		"ShardIteratorType": "TRIM_HORIZON",
	})
	cursor, _ := iterator["ShardIterator"].(string)
	got := raw.call(t, "GetRecords", map[string]any{"ShardIterator": cursor, "Limit": 100})
	rawRecords, _ := got["Records"].([]any)

	shown, err := stack.messages.Query(kinesisContext(t), connID, model.MessageQueryParams{
		Topic:      liveKinesisOrders,
		MaxResults: 200,
		Filters:    map[string]string{kinesisdriver.FilterShardID: shardID},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(shown) != len(rawRecords) {
		t.Fatalf("the board shows %d records on %s, the API returns %d",
			len(shown), shardID, len(rawRecords))
	}

	byID := make(map[string]*model.MessageItem, len(shown))
	for _, message := range shown {
		byID[message.MessageID] = message
	}
	for _, entry := range rawRecords {
		record, _ := entry.(map[string]any)
		sequence, _ := record["SequenceNumber"].(string)
		message := byID[shardID+":"+sequence]
		if message == nil {
			t.Errorf("the API returned %s on %s and the board does not show it", sequence, shardID)
			continue
		}
		encoded, _ := record["Data"].(string)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("decode the record's payload: %v", err)
		}
		if message.Body != string(decoded) {
			t.Errorf("%s: the board shows the body %q, the API carries %q",
				sequence, message.Body, string(decoded))
		}
		key, _ := record["PartitionKey"].(string)
		if message.Keys != key {
			t.Errorf("%s: the board shows the partition key %q, the API says %q",
				sequence, message.Keys, key)
		}
		if message.Properties[kinesisdriver.PropShardID] != shardID {
			t.Errorf("%s: the board attributes the record to %q rather than %q",
				sequence, message.Properties[kinesisdriver.PropShardID], shardID)
		}
	}
}

/*
 * The consumers board, against ListStreamConsumers per stream.
 *
 * The board is one list assembled from a call per stream, so what this catches
 * is a consumer attached to the wrong stream - which nothing on the page would
 * look wrong about, since a name is unique only within its stream and the same
 * name may exist on several.
 */
func TestLiveKinesisCrossCheckConsumersBoard(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis crosscheck consumers"))
	raw := newRawKinesis()

	shown, err := stack.subscriptions.List(kinesisContext(t), connID)
	if err != nil {
		t.Fatalf("List subscriptions: %v", err)
	}
	byPair := make(map[string]*model.Subscription, len(shown))
	for _, entry := range shown {
		byPair[entry.Ref.Namespace+"/"+entry.Ref.Name] = entry
	}

	arn, _ := raw.summary(t, liveKinesisOrders)["StreamARN"].(string)
	out := raw.call(t, "ListStreamConsumers", map[string]any{"StreamARN": arn})
	listed, _ := out["Consumers"].([]any)
	if len(listed) == 0 {
		e2e.Missing(t, "%s has no registered consumers; run `npm run e2e:kinesis:seed`",
			liveKinesisOrders)
	}

	for _, entry := range listed {
		consumer, _ := entry.(map[string]any)
		name, _ := consumer["ConsumerName"].(string)
		row := byPair[liveKinesisOrders+"/"+name]
		if row == nil {
			t.Errorf("the API lists %s on %s and the board does not show it",
				name, liveKinesisOrders)
			continue
		}
		wantARN, _ := consumer["ConsumerARN"].(string)
		if row.Attribute(kinesisdriver.AttrConsumerARN) != wantARN {
			t.Errorf("%s: the board shows arn %q, the API says %q",
				name, row.Attribute(kinesisdriver.AttrConsumerARN), wantARN)
		}
		wantStatus, _ := consumer["ConsumerStatus"].(string)
		if row.Attribute(kinesisdriver.AttrConsumerStatus) != wantStatus {
			t.Errorf("%s: the board shows status %q, the API says %q",
				name, row.Attribute(kinesisdriver.AttrConsumerStatus), wantStatus)
		}
		// The figure that must stay absent whatever else is compared: no call
		// in the API returns a position, so a number here would be invented.
		if row.Backlog != model.UnknownMetric {
			t.Errorf("%s: the board shows a backlog of %d, and nothing reports one",
				name, row.Backlog)
		}
	}

	// The count the streams board carries has to agree with the list, which is
	// two different calls answering the same question.
	detail, err := stack.destinations.Detail(
		kinesisContext(t), connID, model.DestinationRef{Name: liveKinesisOrders})
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.Subscribers != len(listed) {
		t.Errorf("the streams board says %s has %d consumers and the listing has %d",
			liveKinesisOrders, detail.Subscribers, len(listed))
	}
}

/*
 * A send through the app, read back through the raw API.
 *
 * The other direction from the records cross-check, and it is the half that
 * catches a driver reporting a shard or a sequence number it did not actually
 * get: the pair the send returns is used as an address by the raw client,
 * which shares no code with it.
 */
func TestLiveKinesisCrossCheckASendLandsWhereItSaid(t *testing.T) {
	requireLiveKinesis(t)
	stack := newKinesisStack(t)
	connID := stack.dial(t, liveKinesisProfile("kinesis crosscheck send"))
	raw := newRawKinesis()
	name := kinesisTestName(t, "-send")

	testKinesisStreamVia(t, stack, connID, kinesisdriver.StreamSpec{Name: name, Shards: 2})

	result, err := stack.kinesis.Publish(kinesisContext(t), connID, kinesisdriver.PublishRequest{
		Stream: name, Body: "cross-checked", PartitionKey: "cross",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	iterator := raw.call(t, "GetShardIterator", map[string]any{
		"StreamName":             name,
		"ShardId":                result.ShardID,
		"ShardIteratorType":      "AT_SEQUENCE_NUMBER",
		"StartingSequenceNumber": result.SequenceNumber,
	})
	cursor, _ := iterator["ShardIterator"].(string)
	got := raw.call(t, "GetRecords", map[string]any{"ShardIterator": cursor, "Limit": 1})
	records, _ := got["Records"].([]any)
	if len(records) != 1 {
		t.Fatalf("the API found %d records at the sequence number the send reported", len(records))
	}
	record, _ := records[0].(map[string]any)
	encoded, _ := record["Data"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode the record's payload: %v", err)
	}
	if string(decoded) != "cross-checked" {
		t.Errorf("the record at that address holds %q", string(decoded))
	}
	key, _ := record["PartitionKey"].(string)
	if key != "cross" {
		t.Errorf("the record's partition key is %q, want the one the send carried", key)
	}
}
