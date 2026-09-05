package activemq

import (
	"fmt"
	"sort"
	"strings"
)

// product is which of the family's two brokers answered.
//
// One MQKind covers both because they are one family to a user - Artemis is
// where Classic is going, Amazon MQ offers either behind the same console -
// but they agree on nothing a driver reads. Different Jolokia path, different
// MBean domain, different ObjectName keys, different attribute names, and
// message maps whose keys do not overlap at all. Every read in this package
// branches on this value.
type product string

const (
	classic product = "classic"
	artemis product = "artemis"
)

// The MBean domains, which are also how probe tells the products apart: each
// broker registers its tree under its own domain and neither answers for the
// other.
const (
	classicDomain = "org.apache.activemq"
	artemisDomain = "org.apache.activemq.artemis"
)

// The Jolokia agent's path under the web console. Classic mounts it beside its
// REST API, Artemis under the Hawtio console it ships.
const (
	classicPath = "/api/jolokia"
	artemisPath = "/console/jolokia"
)

// objectName builds one JMX ObjectName.
//
// The keys are emitted in alphabetical order, which is what a Jolokia `search`
// returns. JMX itself does not care - a lookup matches on the key set, not on
// their order, and both orderings were confirmed to read the same MBean - but
// matching search output means a name this package built can be compared
// against one the broker reported without normalising either.
type objectName struct {
	domain string
	keys   map[string]string
}

func newObjectName(domain string) *objectName {
	return &objectName{domain: domain, keys: make(map[string]string, 6)}
}

// with adds an unquoted key, for a value the broker itself writes bare.
func (o *objectName) with(key, value string) *objectName {
	o.keys[key] = value
	return o
}

// withQuoted adds a key whose value the broker wraps in double quotes.
//
// Artemis quotes every value in its tree and Classic quotes none of them, so
// which form a key takes is a property of the tree rather than of the value.
// Passing a value through the wrong one produces a name that is syntactically
// fine and matches no MBean, which surfaces as InstanceNotFoundException far
// from the mistake.
func (o *objectName) withQuoted(key, value string) *objectName {
	o.keys[key] = quote(value)
	return o
}

func (o *objectName) String() string {
	names := make([]string, 0, len(o.keys))
	for key := range o.keys {
		names = append(names, key)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, key := range names {
		pairs = append(pairs, key+"="+o.keys[key])
	}
	return o.domain + ":" + strings.Join(pairs, ",")
}

// quote wraps a value the way javax.management.ObjectName.quote does.
//
// Backslash, double quote, newline, asterisk and question mark are escaped;
// everything else passes through. The last two matter more than they look:
// unescaped they turn the name into a pattern, so a queue called "orders?"
// would silently match "ordersX" and the driver would act on the wrong one.
func quote(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"', '*', '?':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// unquote reverses quote for a name read back out of a search result.
func unquote(value string) string {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return value
	}
	inner := value[1 : len(value)-1]

	var b strings.Builder
	b.Grow(len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] != '\\' || i+1 >= len(inner) {
			b.WriteByte(inner[i])
			continue
		}
		i++
		if inner[i] == 'n' {
			b.WriteByte('\n')
			continue
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}

// routingType is Artemis's anycast/multicast distinction, which has no Classic
// equivalent: Classic decides between a queue and a topic by which MBean tree
// the destination sits in, Artemis by a key in the name.
type routingType string

const (
	anycast   routingType = "anycast"
	multicast routingType = "multicast"
)

// destinationKind is the canonical distinction the pages see, which both
// products can express even though they store it differently.
type destinationKind string

const (
	queueKind destinationKind = "queue"
	topicKind destinationKind = "topic"
)

func (r routingType) kind() destinationKind {
	if r == multicast {
		return topicKind
	}
	return queueKind
}

func (k destinationKind) routing() routingType {
	if k == topicKind {
		return multicast
	}
	return anycast
}

// names builds the ObjectNames for one broker's tree.
//
// A value rather than free functions so the broker name is bound once, at
// connect time, instead of being threaded through every call. Which product it
// belongs to decides every method's shape.
type names struct {
	product product
	broker  string
}

// broker is the top of the tree: BrokerViewMBean on Classic, ActiveMQServer
// control on Artemis. Both are the object that creates and destroys
// destinations and reports the broker's own attributes.
func (n names) brokerMBean() string {
	if n.product == artemis {
		return newObjectName(artemisDomain).withQuoted("broker", n.broker).String()
	}
	return newObjectName(classicDomain).
		with("type", "Broker").
		with("brokerName", n.broker).
		String()
}

// destination is one queue or topic.
//
// The two trees disagree about what that even is. Classic addresses a
// destination directly by type and name. Artemis has two levels - an address
// that routes and a queue that stores - so the canonical "destination" is the
// queue, and for an anycast queue the address and queue names are the same
// string. That identity is why one ref can address both products.
func (n names) destination(name string, kind destinationKind) string {
	if n.product == artemis {
		return n.artemisQueue(name, name, kind.routing())
	}
	destinationType := "Queue"
	if kind == topicKind {
		destinationType = "Topic"
	}
	return newObjectName(classicDomain).
		with("type", "Broker").
		with("brokerName", n.broker).
		with("destinationType", destinationType).
		with("destinationName", name).
		String()
}

// artemisQueue names a queue under an address, which is the general form: a
// multicast address carries one queue per durable subscription, and those two
// names differ.
func (n names) artemisQueue(address, queue string, routing routingType) string {
	return newObjectName(artemisDomain).
		withQuoted("address", address).
		withQuoted("broker", n.broker).
		with("component", "addresses").
		withQuoted("queue", queue).
		withQuoted("routing-type", string(routing)).
		with("subcomponent", "queues").
		String()
}

// artemisAddress names the routing level on its own, which is what the
// destinations board reads to learn a topic exists before any subscription has
// created a queue under it.
func (n names) artemisAddress(address string) string {
	return newObjectName(artemisDomain).
		withQuoted("address", address).
		withQuoted("broker", n.broker).
		with("component", "addresses").
		String()
}

// searchPattern is the wildcard this driver hands Jolokia's search command.
//
// Patterns are the only way to enumerate: neither broker has an operation that
// returns its destinations as data, so the list comes from the names of the
// MBeans that exist.
func (n names) searchPattern(component string) string {
	if n.product == artemis {
		return fmt.Sprintf(`%s:broker=%s,component=%s,*`, artemisDomain, quote(n.broker), component)
	}
	return fmt.Sprintf("%s:type=Broker,brokerName=%s,%s,*", classicDomain, n.broker, component)
}

// parseObjectName splits a name the broker reported back into its keys.
//
// Quoted values may contain commas and equals signs, so this cannot be a
// Split: a queue named "a,b" would otherwise come back as two keys, one of
// them malformed.
func parseObjectName(raw string) (domain string, keys map[string]string, err error) {
	colon := strings.Index(raw, ":")
	if colon < 0 {
		return "", nil, fmt.Errorf("object name %q has no domain", raw)
	}
	domain, rest := raw[:colon], raw[colon+1:]
	keys = make(map[string]string, 6)

	for len(rest) > 0 {
		equals := strings.Index(rest, "=")
		if equals < 0 {
			return "", nil, fmt.Errorf("object name %q has a key with no value", raw)
		}
		key := rest[:equals]
		rest = rest[equals+1:]

		var value string
		value, rest = scanValue(rest)
		keys[key] = unquote(value)
	}
	return domain, keys, nil
}

// scanValue reads one value, honouring quotes and the escapes inside them,
// and returns it with whatever follows the separating comma.
func scanValue(rest string) (value, remainder string) {
	if rest == "" || rest[0] != '"' {
		if comma := strings.Index(rest, ","); comma >= 0 {
			return rest[:comma], rest[comma+1:]
		}
		return rest, ""
	}
	for i := 1; i < len(rest); i++ {
		if rest[i] == '\\' {
			i++
			continue
		}
		if rest[i] != '"' {
			continue
		}
		value = rest[:i+1]
		remainder = rest[i+1:]
		return value, strings.TrimPrefix(remainder, ",")
	}
	return rest, ""
}
