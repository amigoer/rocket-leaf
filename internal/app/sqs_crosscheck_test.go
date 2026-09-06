package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every SQS board, compared against the raw API.
 *
 * Almost every figure this family shows is something the driver assembled. A
 * queue's depth is three separate attributes added together; its dead-letter
 * target is a name pulled out of an ARN inside a JSON string; the dead-letter
 * board is the whole topology walked backwards; and a browse is a receive
 * followed by a release, which is arithmetic on delivery state rather than a
 * read. Every one of those can be subtly wrong and stay plausible, and the
 * driver testing itself would produce the same wrong number twice.
 *
 * So the comparison is against a client that shares no code with the driver:
 * its own SigV4, its own request, its own structs. The AWS SDK is deliberately
 * not used here - the driver is a thin layer over it, and using it on both
 * sides would compare the driver against itself.
 *
 * Everything compared exactly is a seeded object, because the driver package's
 * live tests run against the same region and create and delete queues of their
 * own while these are running.
 */

// rawSQS is a minimal SQS client: JSON protocol, SigV4, and nothing else.
type rawSQS struct {
	endpoint string
	region   string
	access   string
	secret   string
	http     *http.Client
}

func newRawSQS() *rawSQS {
	return &rawSQS{
		endpoint: liveSQSEndpoint,
		region:   liveSQSRegion,
		access:   liveSQSAccessKey,
		secret:   liveSQSSecretKey,
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

// call signs one request and decodes its answer.
//
// SigV4 written out rather than imported, which is the whole point of this
// file: a signature the driver's SDK also produced would prove nothing about
// the driver, and the protocol is one POST with four headers.
func (r *rawSQS) call(ctx context.Context, target string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	host, err := url.Parse(r.endpoint)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	stamp := now.Format("20060102T150405Z")
	datestamp := now.Format("20060102")
	bodyHash := sha256.Sum256(body)

	headers := map[string]string{
		"content-type": "application/x-amz-json-1.0",
		"host":         host.Host,
		"x-amz-date":   stamp,
		"x-amz-target": "AmazonSQS." + target,
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	signedHeaders := strings.Join(names, ";")

	var canonicalHeaders strings.Builder
	for _, name := range names {
		fmt.Fprintf(&canonicalHeaders, "%s:%s\n", name, headers[name])
	}
	canonical := strings.Join([]string{
		"POST", "/", "", canonicalHeaders.String(), signedHeaders, hex.EncodeToString(bodyHash[:]),
	}, "\n")
	canonicalHash := sha256.Sum256([]byte(canonical))

	scope := fmt.Sprintf("%s/%s/sqs/aws4_request", datestamp, r.region)
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
	key = sign(key, "sqs")
	key = sign(key, "aws4_request")
	signature := hex.EncodeToString(sign(key, toSign))

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		r.access, scope, signedHeaders, signature))

	response, err := r.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	answer, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("%s: %s: %s", target, response.Status, strings.TrimSpace(string(answer)))
	}
	if out == nil || len(answer) == 0 {
		return nil
	}
	return json.Unmarshal(answer, out)
}

func (r *rawSQS) queueURL(t *testing.T, name string) string {
	t.Helper()
	var answer struct {
		QueueURL string `json:"QueueUrl"`
	}
	if err := r.call(context.Background(), "GetQueueUrl", map[string]any{"QueueName": name}, &answer); err != nil {
		t.Fatalf("raw GetQueueUrl %s: %v", name, err)
	}
	return answer.QueueURL
}

func (r *rawSQS) attributes(t *testing.T, name string) map[string]string {
	t.Helper()
	var answer struct {
		Attributes map[string]string `json:"Attributes"`
	}
	payload := map[string]any{"QueueUrl": r.queueURL(t, name), "AttributeNames": []string{"All"}}
	if err := r.call(context.Background(), "GetQueueAttributes", payload, &answer); err != nil {
		t.Fatalf("raw GetQueueAttributes %s: %v", name, err)
	}
	return answer.Attributes
}

func (r *rawSQS) listQueues(t *testing.T, prefix string) []string {
	t.Helper()
	names := make([]string, 0, 16)
	var token string
	for {
		payload := map[string]any{"MaxResults": 1000}
		if prefix != "" {
			payload["QueueNamePrefix"] = prefix
		}
		if token != "" {
			payload["NextToken"] = token
		}
		var answer struct {
			QueueUrls []string `json:"QueueUrls"`
			NextToken string   `json:"NextToken"`
		}
		if err := r.call(context.Background(), "ListQueues", payload, &answer); err != nil {
			t.Fatalf("raw ListQueues: %v", err)
		}
		for _, queueURL := range answer.QueueUrls {
			names = append(names, queueURL[strings.LastIndex(queueURL, "/")+1:])
		}
		if answer.NextToken == "" {
			break
		}
		token = answer.NextToken
	}
	sort.Strings(names)
	return names
}

func (r *rawSQS) sourceQueues(t *testing.T, name string) []string {
	t.Helper()
	// Lower-cased on this operation alone. That is the service's own wire
	// name, not a typo: the SDK's Go field is QueueUrls and every other
	// operation answers in upper camel, so a struct copied from the one above
	// would silently decode nothing and call every dead-letter queue orphaned.
	var answer struct {
		QueueURLs []string `json:"queueUrls"`
	}
	payload := map[string]any{"QueueUrl": r.queueURL(t, name)}
	if err := r.call(context.Background(), "ListDeadLetterSourceQueues", payload, &answer); err != nil {
		t.Fatalf("raw ListDeadLetterSourceQueues %s: %v", name, err)
	}
	names := make([]string, 0, len(answer.QueueURLs))
	for _, queueURL := range answer.QueueURLs {
		names = append(names, queueURL[strings.LastIndex(queueURL, "/")+1:])
	}
	sort.Strings(names)
	return names
}

func rawInt(t *testing.T, attributes map[string]string, key string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(attributes[key], 10, 64)
	if err != nil {
		t.Fatalf("raw %s = %q, which is not a number", key, attributes[key])
	}
	return value
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
 * The queues board, figure by figure.
 *
 * The depth is the one worth comparing rather than eyeballing: the driver adds
 * three attributes together, and a sum that dropped one would still look like
 * a plausible queue.
 */
func TestLiveSQSCrossCheckQueuesBoard(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)
	connID := stack.dial(t, liveSQSProfile("sqs crosscheck queues"))
	raw := newRawSQS()

	destinations, err := stack.destinations.List(sqsContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}

	for _, name := range []string{liveSQSOrders, liveSQSDLQ, liveSQSDelayed, liveSQSEmpty, liveSQSFIFO} {
		listed := destinationNamed(destinations, name)
		if listed == nil {
			e2e.Missing(t, "%s is not in the app's listing; run `npm run e2e:sqs:seed`", name)
		}
		attributes := raw.attributes(t, name)

		visible := rawInt(t, attributes, "ApproximateNumberOfMessages")
		inFlight := rawInt(t, attributes, "ApproximateNumberOfMessagesNotVisible")
		delayed := rawInt(t, attributes, "ApproximateNumberOfMessagesDelayed")

		if listed.Depth != visible+inFlight+delayed {
			t.Errorf("%s: the app says depth %d and the api says %d+%d+%d",
				name, listed.Depth, visible, inFlight, delayed)
		}
		for _, pair := range []struct {
			key   string
			value int64
		}{
			{"visible", visible},
			{"inFlight", inFlight},
			{"delayed", delayed},
		} {
			if listed.Attribute(pair.key) != strconv.FormatInt(pair.value, 10) {
				t.Errorf("%s: the app says %s=%q and the api says %d",
					name, pair.key, listed.Attribute(pair.key), pair.value)
			}
		}

		// The settings, which reach the queue form and are what an edit sends
		// back: a mis-mapped attribute name here writes the wrong field.
		for _, pair := range []struct{ app, service string }{
			{"visibilityTimeoutSec", "VisibilityTimeout"},
			{"delaySec", "DelaySeconds"},
			{"retentionSec", "MessageRetentionPeriod"},
			{"maxMessageBytes", "MaximumMessageSize"},
			{"receiveWaitSec", "ReceiveMessageWaitTimeSeconds"},
			{"arn", "QueueArn"},
		} {
			if listed.Attribute(pair.app) != attributes[pair.service] {
				t.Errorf("%s: the app says %s=%q and the api says %s=%q",
					name, pair.app, listed.Attribute(pair.app),
					pair.service, attributes[pair.service])
			}
		}

		// FIFO is read off the name rather than from an attribute, so this is
		// the one place the two answers are produced completely differently.
		wantFIFO := strconv.FormatBool(attributes["FifoQueue"] == "true")
		if listed.Attribute("fifo") != wantFIFO {
			t.Errorf("%s: the app says fifo=%q and the api says FifoQueue=%q",
				name, listed.Attribute("fifo"), attributes["FifoQueue"])
		}

		/*
		 * The redrive target, which is the driver's most fragile derivation: a
		 * name pulled out of an ARN inside a JSON string inside an attribute,
		 * whose maxReceiveCount is quoted in the documented shape and
		 * unquoted in some answers.
		 */
		policy := attributes["RedrivePolicy"]
		if policy == "" {
			if listed.Attribute("deadLetterQueue") != "" {
				t.Errorf("%s: the app names a dead-letter queue %q and the api has no redrive policy",
					name, listed.Attribute("deadLetterQueue"))
			}
			continue
		}
		var decoded struct {
			DeadLetterTargetArn string `json:"deadLetterTargetArn"`
		}
		if err := json.Unmarshal([]byte(policy), &decoded); err != nil {
			t.Fatalf("%s: the api's redrive policy is not json: %v", name, err)
		}
		wantTarget := decoded.DeadLetterTargetArn[strings.LastIndex(decoded.DeadLetterTargetArn, ":")+1:]
		if listed.Attribute("deadLetterQueue") != wantTarget {
			t.Errorf("%s: the app says the dead letters go to %q and the arn says %q",
				name, listed.Attribute("deadLetterQueue"), wantTarget)
		}
	}
}

/*
 * The listing itself, and the prefix that narrows it.
 *
 * Compared against the same prefix rather than against everything, because the
 * driver package's live tests create and delete MQS-TEST- queues on this same
 * region while this runs - the seeded set is the only stable one.
 */
func TestLiveSQSCrossCheckTheListingHonoursThePrefix(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)

	profile := liveSQSProfile("sqs crosscheck prefix")
	profile.Options[sqsdriver.OptionQueuePrefix] = "MQS-SEED-"
	connID := stack.dial(t, profile)

	destinations, err := stack.destinations.List(sqsContext(t), connID, model.DestinationFilter{})
	if err != nil {
		t.Fatalf("list destinations: %v", err)
	}
	appNames := make([]string, 0, len(destinations))
	for _, entry := range destinations {
		appNames = append(appNames, entry.Ref.Name)
	}
	sort.Strings(appNames)

	apiNames := newRawSQS().listQueues(t, "MQS-SEED-")
	if len(apiNames) == 0 {
		e2e.Missing(t, "the region holds no MQS-SEED- queues; run `npm run e2e:sqs:seed`")
	}
	if !slices.Equal(appNames, apiNames) {
		t.Errorf("the app lists %v and the api lists %v", appNames, apiNames)
	}
}

/*
 * The dead-letter board, which is the driver's own topology walk.
 *
 * Nothing in SQS marks a dead-letter queue, so the app derives the set from
 * every other queue's redrive policy. The comparison here builds it the other
 * way round - from ListDeadLetterSourceQueues, the service's own answer -
 * which is exactly what a walk that missed a queue would still agree with
 * itself about.
 */
func TestLiveSQSCrossCheckDeadLetterBoard(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)

	profile := liveSQSProfile("sqs crosscheck dead letters")
	profile.Options[sqsdriver.OptionQueuePrefix] = "MQS-SEED-"
	connID := stack.dial(t, profile)
	raw := newRawSQS()

	queues, err := stack.sqs.DeadLetterQueues(sqsContext(t), connID)
	if err != nil {
		t.Fatalf("dead letter queues: %v", err)
	}

	appNames := make([]string, 0, len(queues))
	for _, queue := range queues {
		appNames = append(appNames, queue.Name)
	}
	sort.Strings(appNames)

	// Built independently: every seeded queue the api says has sources.
	apiNames := make([]string, 0, 4)
	for _, name := range raw.listQueues(t, "MQS-SEED-") {
		if len(raw.sourceQueues(t, name)) > 0 {
			apiNames = append(apiNames, name)
		}
	}
	sort.Strings(apiNames)
	if len(apiNames) == 0 {
		e2e.Missing(t, "no seeded queue redrives anywhere; run `npm run e2e:sqs:seed`")
	}
	if !slices.Equal(appNames, apiNames) {
		t.Errorf("the app calls %v dead-letter queues and the api says %v", appNames, apiNames)
	}

	for _, queue := range queues {
		sources := make([]string, 0, len(queue.Sources))
		for _, source := range queue.Sources {
			sources = append(sources, source.Queue)
		}
		sort.Strings(sources)
		if want := raw.sourceQueues(t, queue.Name); !slices.Equal(sources, want) {
			t.Errorf("%s: the app says %v redrives here and the api says %v",
				queue.Name, sources, want)
		}

		attributes := raw.attributes(t, queue.Name)
		want := rawInt(t, attributes, "ApproximateNumberOfMessages") +
			rawInt(t, attributes, "ApproximateNumberOfMessagesNotVisible") +
			rawInt(t, attributes, "ApproximateNumberOfMessagesDelayed")
		if queue.Depth != want {
			t.Errorf("%s: the app says depth %d and the api says %d", queue.Name, queue.Depth, want)
		}
	}
}

/*
 * The messages board, and the one thing about it that has to be true after the
 * page has been drawn.
 *
 * A browse is a receive followed by a release, so the comparison is not only
 * "did the bodies match" but "is the queue as available afterwards as it was
 * before". The queue is the test's own, because the assertion is about what
 * the browse did to it - running it against a seeded queue would take
 * messages away from every other test on this region.
 */
func TestLiveSQSCrossCheckBrowseReturnsWhatItTook(t *testing.T) {
	requireLiveSQS(t)
	stack := newSQSStack(t)
	connID := stack.dial(t, liveSQSProfile("sqs crosscheck browse"))
	raw := newRawSQS()

	name := sqsTestName(t, "")
	// Five minutes, so a message the driver failed to release would still be
	// invisible when this looks - the timeout cannot be what puts it back.
	testQueueVia(t, stack, connID, sqsdriver.QueueSpec{Name: name, VisibilityTimeoutSec: 300})

	if _, err := stack.sqs.Publish(sqsContext(t), connID, sqsdriver.PublishRequest{
		Queue: name, Body: "crosscheck", Count: 6,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	waitForRawDepth(t, raw, name, 6)

	messages, err := stack.messages.Query(sqsContext(t), connID, model.MessageQueryParams{
		Topic: name, MaxResults: 6,
	})
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	if len(messages) != 6 {
		t.Fatalf("the app browsed %d messages and the api was sent 6", len(messages))
	}
	for _, item := range messages {
		if item.Body != "crosscheck" {
			t.Errorf("body = %q, want crosscheck", item.Body)
		}
	}

	// Available again, which the visibility timeout cannot explain.
	waitForRawDepth(t, raw, name, 6)
	attributes := raw.attributes(t, name)
	if got := rawInt(t, attributes, "ApproximateNumberOfMessagesNotVisible"); got != 0 {
		t.Errorf("the api reports %d messages still in flight after the browse; "+
			"the driver did not hand them back", got)
	}
}

// waitForRawDepth waits for the api's own count, because every SQS figure is
// what its servers last agreed on rather than what is true this instant.
func waitForRawDepth(t *testing.T, raw *rawSQS, name string, want int64) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		got := rawInt(t, raw.attributes(t, name), "ApproximateNumberOfMessages")
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the api reports %d available on %s after 30s, want %d", got, name, want)
		}
		time.Sleep(time.Second)
	}
}
