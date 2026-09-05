package activemq

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// There is no embeddable ActiveMQ: both products are Java, and Go has no
// equivalent of the nats-server, kfake, miniredis or mochi-mqtt the other
// drivers test against. So the offline fixture is an HTTP server answering
// with JSON captured from the real brokers, and the risk that comes with it -
// a transcript can drift from the broker it was taken from and these tests
// stay green - is what the cross-check against raw Jolokia exists to cover.
func jolokiaFixture(t *testing.T, handler http.HandlerFunc) *jolokiaClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := newJolokiaClient(server.URL, artemisPath, "artemis", "artemis", "", time.Second, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

// The trap this driver would otherwise have shipped with.
//
// Both brokers ship jolokia-access.xml with <strict-checking/>, which refuses
// a request carrying no Origin header as coming from the null origin. The
// refusal is an HTTP 403 whose body reads "Origin null is not allowed to call
// this agent" - which looks exactly like bad credentials, and would have sent
// everyone who hit it to re-check a password that was fine.
func TestEveryRequestCarriesAnOriginHeader(t *testing.T) {
	var seen string
	client := jolokiaFixture(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Origin")
		writeResults(t, w, `[{"status":200,"value":"2.44.0"}]`)
	})

	if _, err := client.readString(context.Background(), "mbean", "Version"); err != nil {
		t.Fatalf("read: %v", err)
	}
	if seen != defaultOrigin {
		t.Errorf("Origin = %q, want %q", seen, defaultOrigin)
	}
}

func TestAnExplicitOriginOverridesTheDefault(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Origin")
		writeResults(t, w, `[{"status":200,"value":"6.2.0"}]`)
	}))
	t.Cleanup(server.Close)

	client, err := newJolokiaClient(server.URL, classicPath, "", "", "https://console.example", time.Second, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	if _, err := client.readString(context.Background(), "mbean", "BrokerVersion"); err != nil {
		t.Fatalf("read: %v", err)
	}
	if seen != "https://console.example" {
		t.Errorf("Origin = %q", seen)
	}
}

// The second trap: Jolokia answers HTTP 200 and puts the real outcome in the
// body. A client that checks only the transport reports every broker-side
// failure - a missing queue, a refused operation, an exception thrown inside
// the MBean - as success with an empty value.
func TestABrokerSideFailureIsNotReportedAsSuccess(t *testing.T) {
	client := jolokiaFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResults(t, w, `[{"status":404,"error_type":"javax.management.InstanceNotFoundException",`+
			`"error":"javax.management.InstanceNotFoundException : no such queue"}]`)
	})

	_, err := client.call(context.Background(), readAttribute("gone", "MessageCount"))
	if err == nil {
		t.Fatal("a 404 in the body was reported as success")
	}
	if !notRegistered(err) {
		t.Errorf("error not recognised as a missing mbean: %v", err)
	}
}

func TestAPolicyRefusalIsToldApartFromEveryOtherFailure(t *testing.T) {
	client := jolokiaFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResults(t, w, `[{"status":403,"error_type":"java.lang.Exception",`+
			`"error":"java.lang.Exception : Origin null is not allowed to call this agent"}]`)
	})

	_, err := client.call(context.Background(), readAttribute("mbean", "Version"))
	if !forbidden(err) {
		t.Errorf("a 403 was not recognised as a refusal: %v", err)
	}
	if notRegistered(err) {
		t.Error("a refusal was also read as a missing mbean")
	}
}

// Batching is what keeps a board that reads twenty queues to one round trip.
// The agent answers in request order and this asserts the pairing, because a
// silent reordering would attach every queue's depth to its neighbour.
func TestABatchKeepsItsOrder(t *testing.T) {
	var sent []request
	client := jolokiaFixture(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Errorf("body was not an array of requests: %v", err)
		}
		writeResults(t, w, `[{"status":200,"value":1},{"status":200,"value":2},{"status":200,"value":3}]`)
	})

	values, err := client.batch(context.Background(), []request{
		readAttribute("a", "MessageCount"),
		readAttribute("b", "MessageCount"),
		readAttribute("c", "MessageCount"),
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(sent) != 3 || sent[0].MBean != "a" || sent[2].MBean != "c" {
		t.Errorf("requests were not sent as one ordered array: %+v", sent)
	}
	for i, want := range []string{"1", "2", "3"} {
		if string(values[i]) != want {
			t.Errorf("value %d = %s, want %s", i, values[i], want)
		}
	}
}

// A board reading many destinations races a deletion between the search that
// found them and the read that measures them, so one member failing must not
// cost the other nineteen their answer.
func TestATolerantBatchReportsPerMemberFailures(t *testing.T) {
	client := jolokiaFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResults(t, w, `[{"status":200,"value":7},`+
			`{"status":404,"error_type":"javax.management.InstanceNotFoundException","error":"gone"}]`)
	})

	values, errs, err := client.batchTolerant(context.Background(), []request{
		readAttribute("here", "MessageCount"),
		readAttribute("gone", "MessageCount"),
	})
	if err != nil {
		t.Fatalf("batchTolerant: %v", err)
	}
	if string(values[0]) != "7" {
		t.Errorf("surviving member lost its value: %s", values[0])
	}
	if errs[0] != nil {
		t.Errorf("surviving member carried an error: %v", errs[0])
	}
	if !notRegistered(errs[1]) {
		t.Errorf("missing member was not reported: %v", errs[1])
	}
}

// Some agent versions answer a one-element batch with a bare object rather
// than an array of one, and a decoder that insists on an array turns that into
// a parse failure on a call that worked.
func TestASingleResultIsAcceptedAsAnObjectOrAnArray(t *testing.T) {
	client := jolokiaFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResults(t, w, `{"status":200,"value":"6.2.0"}`)
	})

	got, err := client.readString(context.Background(), "mbean", "BrokerVersion")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "6.2.0" {
		t.Errorf("value = %q", got)
	}
}

// A search that matched nothing is how both brokers say "no destinations", not
// a failure - a broker with no queues would otherwise show an error page.
func TestASearchThatMatchedNothingIsNotAnError(t *testing.T) {
	client := jolokiaFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		writeResults(t, w, `[{"status":404,"error_type":"javax.management.InstanceNotFoundException","error":"none"}]`)
	})

	found, err := client.search(context.Background(), "org.apache.activemq.artemis:broker=*")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("found = %v", found)
	}
}

// An operation taking no arguments must not carry an empty array: Jolokia
// rejects "arguments": [] on a no-arg operation, which would break purge and
// browse - the two most-used calls in the driver.
func TestANoArgumentOperationOmitsTheArgumentsField(t *testing.T) {
	encoded, err := json.Marshal(execOperation("mbean", "browse()"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(encoded); got != `{"type":"exec","mbean":"mbean","operation":"browse()"}` {
		t.Errorf("encoded = %s", got)
	}
}

func writeResults(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write: %v", err)
	}
}
