package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	activemqdriver "github.com/amigoer/mq-studio/internal/driver/activemq"
	"github.com/amigoer/mq-studio/internal/e2e"
	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Every ActiveMQ board against raw Jolokia.
 *
 * The other live tests compare one library call against another and can only
 * show that the two agree. This compares what the app computes against the
 * broker's own answer, fetched by a client that shares no code with the driver
 * - its own HTTP request, its own ObjectNames, its own parsing.
 *
 * That matters more for this family than for any other here. There is no
 * embeddable ActiveMQ: both products are Java, and Go has nothing like the
 * nats-server, kfake, miniredis or mochi-mqtt the other drivers test against.
 * So the offline fixtures are recorded JSON, and a recording can drift from
 * the broker it was taken from while every offline test stays green. This is
 * the only thing that would notice.
 *
 * Both products, because they share no ObjectName, no attribute name and no
 * message-map key - a crosscheck green against one proves nothing about the
 * other.
 */

const (
	crossArtemisConsole = "http://127.0.0.1:8161/console/jolokia"
	crossArtemisAuth    = "artemis:artemis"
	crossClassicConsole = "http://127.0.0.1:8162/api/jolokia"
	crossClassicAuth    = "admin:admin"
	crossClassicProbe   = "http://127.0.0.1:8162/api/jolokia/search/org.apache.activemq:type=Broker,brokerName=*"
)

func requireLiveClassic(t *testing.T) {
	t.Helper()
	e2e.Require(t, e2e.Env{
		Family: e2e.ActiveMQ,
		Name:   "the activemq classic e2e broker",
		Start:  "npm run e2e:activemq:classic:up",
		Probe:  e2e.HTTPGet(crossClassicProbe),
	})
}

// jolokiaProbe is a Jolokia client that shares nothing with the driver's.
//
// Deliberately its own thirty lines rather than a call into internal/driver:
// a crosscheck that reused the driver's client would compare the driver
// against itself, which is what these tests exist not to do. The Origin header
// is here for the same reason the driver has one - both brokers refuse a
// request without it - and that is a fact about the brokers rather than about
// the driver, so repeating it is correct.
type jolokiaProbe struct {
	endpoint string
	auth     string
}

func (p jolokiaProbe) read(t *testing.T, mbean, attribute string) json.RawMessage {
	t.Helper()
	return p.post(t, map[string]any{"type": "read", "mbean": mbean, "attribute": attribute})
}

func (p jolokiaProbe) exec(t *testing.T, mbean, operation string, arguments ...any) json.RawMessage {
	t.Helper()
	request := map[string]any{"type": "exec", "mbean": mbean, "operation": operation}
	if len(arguments) > 0 {
		request["arguments"] = arguments
	}
	return p.post(t, request)
}

func (p jolokiaProbe) post(t *testing.T, request map[string]any) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Origin", "http://localhost")
	httpReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(p.auth)))

	response, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("jolokia: %v", err)
	}
	defer func() { _ = response.Body.Close() }()

	var answer struct {
		Status int             `json:"status"`
		Value  json.RawMessage `json:"value"`
		Error  string          `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if answer.Status != http.StatusOK {
		t.Fatalf("jolokia %d on %v: %s", answer.Status, request["mbean"], answer.Error)
	}
	return answer.Value
}

func (p jolokiaProbe) number(t *testing.T, mbean, attribute string) int64 {
	t.Helper()
	var value float64
	if err := json.Unmarshal(p.read(t, mbean, attribute), &value); err != nil {
		t.Fatalf("%s.%s is not a number: %v", mbean, attribute, err)
	}
	return int64(value)
}

// crossProduct is one broker to check, with the ObjectNames written out here
// rather than built by the driver's objectname.go.
type crossProduct struct {
	name    string
	require func(*testing.T)
	profile func(string) model.ConnectionProfile
	probe   jolokiaProbe
	broker  string
	// queue names the MBean of the seeded orders queue, and depth the
	// attribute holding its message count. Both differ entirely between the
	// products, which is the whole reason this table exists.
	queue string
	depth string
}

func crossProducts() []crossProduct {
	return []crossProduct{
		{
			name:    "artemis",
			require: requireLiveActiveMQ,
			profile: func(name string) model.ConnectionProfile {
				return liveActiveMQProfile(name, false)
			},
			probe:  jolokiaProbe{endpoint: crossArtemisConsole, auth: crossArtemisAuth},
			broker: `org.apache.activemq.artemis:broker="0.0.0.0"`,
			queue: `org.apache.activemq.artemis:address="MQS.SEED.orders",broker="0.0.0.0",` +
				`component=addresses,queue="MQS.SEED.orders",routing-type="anycast",subcomponent=queues`,
			depth: "MessageCount",
		},
		{
			name:    "classic",
			require: requireLiveClassic,
			profile: func(name string) model.ConnectionProfile {
				profile := liveActiveMQProfile(name, false)
				profile.Endpoints = "http://127.0.0.1:8162"
				profile.Secrets[activemqdriver.SecretUsername] = "admin"
				profile.Secrets[activemqdriver.SecretPassword] = "admin"
				return profile
			},
			probe:  jolokiaProbe{endpoint: crossClassicConsole, auth: crossClassicAuth},
			broker: "org.apache.activemq:type=Broker,brokerName=localhost",
			queue: "org.apache.activemq:type=Broker,brokerName=localhost," +
				"destinationType=Queue,destinationName=MQS.SEED.orders",
			depth: "QueueSize",
		},
	}
}

// The destinations board's figures against the broker's own.
func TestLiveActiveMQDestinationsAgreeWithJolokia(t *testing.T) {
	for _, product := range crossProducts() {
		t.Run(product.name, func(t *testing.T) {
			product.require(t)
			stack := newActiveMQStack(t)
			connID := stack.dial(t, product.profile("cross-destinations-"+product.name))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			destinations, err := stack.destinations.List(ctx, connID, model.DestinationFilter{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			var orders *model.Destination
			for _, entry := range destinations {
				if entry.Ref.Name == "MQS.SEED.orders" {
					orders = entry
				}
			}
			if orders == nil {
				t.Skip("the seed has not run")
			}

			want := product.probe.number(t, product.queue, product.depth)
			if orders.Depth != want {
				t.Errorf("the board says depth %d and the broker says %d", orders.Depth, want)
			}

			// The count, compared against the figure that actually means the
			// same thing on each product - which is not the same attribute.
			//
			// Classic counts queues and topics separately and its topic count
			// includes the advisory topics the board hides, so the exact
			// comparison is queues against queues. Artemis counts addresses
			// including its own internal ones, which the board also hides, so
			// there the honest assertion is that the board lists no more than
			// the broker has.
			if product.name == "classic" {
				queues := 0
				for _, entry := range destinations {
					if entry.Attributes[activemqdriver.AttrKind] == "queue" {
						queues++
					}
				}
				reported := product.probe.number(t, product.broker, "TotalManagedQueuesCount")
				if int64(queues) != reported {
					t.Errorf("the board lists %d queues and the broker counts %d",
						queues, reported)
				}
			} else {
				reported := product.probe.number(t, product.broker, "AddressCount")
				if int64(len(destinations)) > reported {
					t.Errorf("the board lists %d destinations and the broker counts %d addresses",
						len(destinations), reported)
				}
			}
		})
	}
}

// The message board's browse against the broker's own browse.
//
// The same operation through two entirely different paths - the driver's
// client and its own parsing on one side, thirty lines of net/http on the
// other - which is what makes a drifted fixture visible.
func TestLiveActiveMQBrowseAgreesWithJolokia(t *testing.T) {
	for _, product := range crossProducts() {
		t.Run(product.name, func(t *testing.T) {
			product.require(t)
			stack := newActiveMQStack(t)
			connID := stack.dial(t, product.profile("cross-browse-"+product.name))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			queue := "MQS.CROSS." + product.name
			_ = stack.activemq.RemoveDestination(ctx, connID, queue)
			if err := stack.activemq.CreateDestination(ctx, connID, queue, false); err != nil {
				t.Fatalf("create: %v", err)
			}
			t.Cleanup(func() {
				_ = stack.activemq.RemoveDestination(context.Background(), connID, queue)
			})

			const sent = 4
			if _, err := stack.activemq.Publish(ctx, connID, model.PublishRequest{
				RoutingKey: queue,
				Body:       "cross-check",
				Persistent: true,
				Count:      sent,
			}); err != nil {
				t.Fatalf("Publish: %v", err)
			}

			messages, err := stack.messages.Query(ctx, connID, model.MessageQueryParams{Topic: queue})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(messages) != sent {
				t.Fatalf("the board browsed %d messages, want %d", len(messages), sent)
			}

			mbean := product.queue
			if product.name == "artemis" {
				mbean = fmt.Sprintf(
					`org.apache.activemq.artemis:address=%q,broker="0.0.0.0",component=addresses,`+
						`queue=%q,routing-type="anycast",subcomponent=queues`, queue, queue)
			} else {
				mbean = "org.apache.activemq:type=Broker,brokerName=localhost," +
					"destinationType=Queue,destinationName=" + queue
			}

			var raw []map[string]json.RawMessage
			if err := json.Unmarshal(product.probe.exec(t, mbean, "browse()"), &raw); err != nil {
				t.Fatalf("the raw browse did not parse: %v", err)
			}
			if len(raw) != len(messages) {
				t.Errorf("the board browsed %d and the broker returned %d",
					len(messages), len(raw))
			}

			// And the bodies, which is where the two products' unrelated key
			// sets would show up: Classic answers with Text and Artemis with
			// text, and a driver reading the wrong one returns empty strings
			// and no error at all.
			bodyKey := "text"
			if product.name == "classic" {
				bodyKey = "Text"
			}
			for _, entry := range raw {
				var body string
				if err := json.Unmarshal(entry[bodyKey], &body); err != nil {
					t.Fatalf("the raw browse has no %s: %v", bodyKey, err)
				}
				if body != "cross-check" {
					t.Errorf("the broker's own body is %q", body)
				}
			}
			for _, message := range messages {
				if message.Body != "cross-check" {
					t.Errorf("the board's body is %q", message.Body)
				}
			}
		})
	}
}

// The broker board's figures against the broker's own.
func TestLiveActiveMQBrokerAgreesWithJolokia(t *testing.T) {
	for _, product := range crossProducts() {
		t.Run(product.name, func(t *testing.T) {
			product.require(t)
			stack := newActiveMQStack(t)
			connID := stack.dial(t, product.profile("cross-broker-"+product.name))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			nodes, err := stack.activemq.Nodes(ctx, connID)
			if err != nil {
				t.Fatalf("Nodes: %v", err)
			}
			if len(nodes) == 0 {
				t.Fatal("the broker board is empty")
			}
			broker := nodes[0]

			versionAttribute := "Version"
			if product.name == "classic" {
				versionAttribute = "BrokerVersion"
			}
			var version string
			if err := json.Unmarshal(
				product.probe.read(t, product.broker, versionAttribute), &version); err != nil {
				t.Fatalf("version: %v", err)
			}
			if broker.Version != version {
				t.Errorf("the board says version %q and the broker says %q",
					broker.Version, version)
			}

			messages := product.probe.number(t, product.broker, "TotalMessageCount")
			reported := broker.Attributes["totalMessages"]
			if reported != fmt.Sprint(messages) {
				t.Errorf("the board says %s messages held and the broker says %d",
					reported, messages)
			}
		})
	}
}
