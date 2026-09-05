package activemq

import (
	"testing"
)

// The two trees, side by side.
//
// This is the table the rest of the driver rests on: a name built for the
// wrong tree is syntactically valid, matches no MBean, and surfaces as
// InstanceNotFoundException in whichever page happened to ask - a long way
// from the mistake. Both products' real names were taken from a Jolokia
// search against 6.2.0 and 2.44.0 rather than from documentation.
func TestDestinationNamesMatchEachProductsTree(t *testing.T) {
	cases := []struct {
		name  string
		names names
		queue string
		kind  destinationKind
		want  string
	}{
		{
			name:  "classic queue",
			names: names{product: classic, broker: "localhost"},
			queue: "MQS.SEED.orders",
			kind:  queueKind,
			want: "org.apache.activemq:brokerName=localhost," +
				"destinationName=MQS.SEED.orders,destinationType=Queue,type=Broker",
		},
		{
			name:  "classic topic",
			names: names{product: classic, broker: "localhost"},
			queue: "MQS.SEED.events",
			kind:  topicKind,
			want: "org.apache.activemq:brokerName=localhost," +
				"destinationName=MQS.SEED.events,destinationType=Topic,type=Broker",
		},
		{
			// An anycast queue's address and queue names are the same string,
			// which is what lets one canonical ref address both products.
			name:  "artemis anycast queue",
			names: names{product: artemis, broker: "0.0.0.0"},
			queue: "MQS.SEED.orders",
			kind:  queueKind,
			want: `org.apache.activemq.artemis:address="MQS.SEED.orders",broker="0.0.0.0",` +
				`component=addresses,queue="MQS.SEED.orders",routing-type="anycast",subcomponent=queues`,
		},
		{
			name:  "artemis multicast address",
			names: names{product: artemis, broker: "0.0.0.0"},
			queue: "MQS.SEED.events",
			kind:  topicKind,
			want: `org.apache.activemq.artemis:address="MQS.SEED.events",broker="0.0.0.0",` +
				`component=addresses,queue="MQS.SEED.events",routing-type="multicast",subcomponent=queues`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.names.destination(tc.queue, tc.kind); got != tc.want {
				t.Errorf("destination name\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// A durable subscription on Artemis is a queue whose name differs from the
// address above it, which is the one shape the anycast identity above does not
// cover.
func TestArtemisSubscriptionNamesItsAddressAndQueueSeparately(t *testing.T) {
	n := names{product: artemis, broker: "0.0.0.0"}
	want := `org.apache.activemq.artemis:address="MQS.SEED.events",broker="0.0.0.0",` +
		`component=addresses,queue="MQS.SEED.events.analytics",routing-type="multicast",subcomponent=queues`
	if got := n.artemisQueue("MQS.SEED.events", "MQS.SEED.events.analytics", multicast); got != want {
		t.Errorf("subscription name\n got: %s\nwant: %s", got, want)
	}
}

// Keys come out in alphabetical order because that is what a Jolokia search
// returns, so a name this package built can be compared against one the broker
// reported without normalising either.
func TestKeysAreEmittedAlphabetically(t *testing.T) {
	got := newObjectName("d").with("zebra", "1").with("alpha", "2").with("mid", "3").String()
	if want := "d:alpha=2,mid=3,zebra=1"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// Unescaped, a wildcard turns a name into a pattern - so a queue really called
// "orders?" would match "ordersX" and the driver would purge the wrong one.
func TestQuoteEscapesTheCharactersThatWouldMakeAPattern(t *testing.T) {
	cases := map[string]string{
		"plain":      `"plain"`,
		"orders?":    `"orders\?"`,
		"orders*":    `"orders\*"`,
		`say"what`:   `"say\"what"`,
		`back\lash`:  `"back\\lash"`,
		"two\nlines": `"two\nlines"`,
	}
	for in, want := range cases {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
		if round := unquote(quote(in)); round != in {
			t.Errorf("unquote(quote(%q)) = %q", in, round)
		}
	}
}

// A quoted value may hold a comma or an equals sign, so splitting on either
// would turn a queue called "a,b" into two keys and one of them malformed.
func TestParseObjectNameHonoursQuotedSeparators(t *testing.T) {
	raw := `org.apache.activemq.artemis:address="a,b=c",broker="0.0.0.0",component=addresses`
	domain, keys, err := parseObjectName(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if domain != artemisDomain {
		t.Errorf("domain = %q", domain)
	}
	if keys["address"] != "a,b=c" {
		t.Errorf("address = %q, want %q", keys["address"], "a,b=c")
	}
	if keys["component"] != "addresses" {
		t.Errorf("component = %q", keys["component"])
	}
}

// The names a real search returned, parsed back. Both products, because the
// key that carries the broker name differs and picking the wrong one leaves
// the driver with no broker at all.
func TestBrokerNameIsReadOffASearchResult(t *testing.T) {
	artemisFound := []string{
		`org.apache.activemq.artemis:broker="0.0.0.0"`,
		`org.apache.activemq.artemis:broker="0.0.0.0",component=acceptors,name="amqp"`,
	}
	if name, ok := brokerNameFrom(artemisFound, artemis); !ok || name != "0.0.0.0" {
		t.Errorf("artemis broker = %q, %v", name, ok)
	}

	classicFound := []string{"org.apache.activemq:brokerName=localhost,type=Broker"}
	if name, ok := brokerNameFrom(classicFound, classic); !ok || name != "localhost" {
		t.Errorf("classic broker = %q, %v", name, ok)
	}
}

// One JVM can register several brokers and nothing says which a profile meant,
// so this reports it rather than picking - the form has a field for it.
func TestSeveralBrokersIsNotResolvedBySilentlyPickingOne(t *testing.T) {
	found := []string{
		"org.apache.activemq:brokerName=one,type=Broker",
		"org.apache.activemq:brokerName=two,type=Broker",
	}
	if name, ok := brokerNameFrom(found, classic); ok {
		t.Errorf("picked %q from two brokers", name)
	}
}
