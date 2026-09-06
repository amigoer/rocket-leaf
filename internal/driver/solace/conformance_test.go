package solace

import (
	"strings"
	"testing"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// offlineConn is a connection with the family's declared capabilities and no
// broker behind it. Conformance is a question about the type, not about a
// server, so it must be answerable with nothing running.
func offlineConn() *Conn {
	conn := &Conn{vpn: "default", closed: make(chan struct{})}
	conn.capabilities = model.NewCapabilities(capabilities()...)
	return conn
}

// The UI gates on the capability list and Go gates on the interfaces. Nothing
// in the language forces those to agree, so this is what turns a disagreement
// into a build failure instead of a control that does nothing when clicked.
func TestConnDeclaresOnlyWhatItImplements(t *testing.T) {
	for _, problem := range driver.CheckConformance(offlineConn()) {
		t.Error(problem)
	}
}

// The descriptor is read before anything is dialled, so it has to stand on its
// own: a form that writes into a target nothing reads, or a capability the
// connection cannot honour, would both surface as a dead control.
func TestDescriptorIsSelfConsistent(t *testing.T) {
	descriptor := New().Descriptor()

	if descriptor.Kind != model.KindSolace {
		t.Errorf("kind = %q, want solace", descriptor.Kind)
	}
	if len(descriptor.Form) == 0 {
		t.Fatal("descriptor carries no connection form")
	}

	keys := make(map[string]bool, len(descriptor.Form))
	for _, field := range descriptor.Form {
		if field.Key == "" || field.LabelKey == "" {
			t.Errorf("form field is missing a key or label: %#v", field)
		}
		if keys[field.Key] {
			t.Errorf("form field %q is declared twice", field.Key)
		}
		keys[field.Key] = true
		switch field.Target {
		case model.TargetEndpoints, model.TargetOption, model.TargetSecret, model.TargetAuth:
		default:
			t.Errorf("form field %q writes into an unknown target %q", field.Key, field.Target)
		}
		if field.Type == model.FieldSelect && len(field.Options) == 0 {
			t.Errorf("form field %q is a select with no options", field.Key)
		}
	}

	for _, field := range descriptor.Form {
		if field.VisibleWhen == nil {
			continue
		}
		if !keys[field.VisibleWhen.Field] {
			t.Errorf("form field %q is shown by %q, which is not on the form",
				field.Key, field.VisibleWhen.Field)
		}
		if len(field.VisibleWhen.Equals) == 0 {
			t.Errorf("form field %q has a condition that matches nothing", field.Key)
		}
	}

	live := offlineConn().Capabilities()
	for _, capability := range descriptor.MaxCapabilities {
		if live.Has(capability) {
			continue
		}
		if reason, degraded := live.DegradedReason(capability); degraded {
			if reason == "" {
				t.Errorf("%s is degraded with no reason to show", capability)
			}
			continue
		}
		t.Errorf("descriptor promises %s but a connection neither supports nor degrades it", capability)
	}
}

/*
 * This family has an address, and it has to keep saying so.
 *
 * It would be easy to file Solace with the hosted families on the strength of
 * Solace Cloud, the way Service Bus and Kinesis were nearly filed. But what a
 * profile carries is http://host:8080 - a DNS host and a TCP port a user types
 * into the form - and every SEMP path is built on it.
 * model.DriverDescriptor.RequiresEndpoints reads the form rather than a list
 * of kinds, so a required endpoint field here is what makes the connection
 * service demand an address, and dropping it would let a profile save with
 * nothing to dial.
 */
func TestDescriptorAsksForAnAddress(t *testing.T) {
	descriptor := New().Descriptor()

	var endpoints *model.FormField
	for index, field := range descriptor.Form {
		if field.Target == model.TargetEndpoints {
			endpoints = &descriptor.Form[index]
		}
	}
	if endpoints == nil {
		t.Fatal("the form asks for no address, and the broker's semp port is one this driver dials")
	}
	if !endpoints.Required {
		t.Error("the address is optional, and there is nothing this driver could derive it from")
	}
	if !descriptor.RequiresEndpoints() {
		t.Error("the descriptor does not demand an address, so a profile could save with none")
	}

	// The Message VPN is not the address and must not be collected as one: it
	// is a path segment on the broker the endpoint names, and the shell
	// re-points it without touching the address at all.
	for _, field := range descriptor.Form {
		if field.Key == OptionMsgVPN && field.Target != model.TargetOption {
			t.Errorf("the message vpn writes into %q; it is a scope, not an address", field.Target)
		}
	}
}

/*
 * The Message VPN is the scope, and the descriptor is where the shell learns
 * its name.
 *
 * ScopeOption is what internal/bridge/connection.go writes when the switcher
 * changes scope, so the shell never has to know that Solace spells it
 * "msgVpn". A descriptor that declared the capability and left this empty
 * would draw the switcher and refuse every switch.
 */
func TestDescriptorNamesTheScopeOption(t *testing.T) {
	descriptor := New().Descriptor()

	if descriptor.ScopeOption != OptionMsgVPN {
		t.Errorf("scope option = %q, want %q", descriptor.ScopeOption, OptionMsgVPN)
	}
	onForm := false
	for _, field := range descriptor.Form {
		if field.Key == descriptor.ScopeOption {
			onForm = true
		}
	}
	if !onForm {
		t.Error("the scope option is not a field on the form, so a profile could never set it by hand")
	}
}

/*
 * The credentials are this driver's own names, and they must never be
 * RocketMQ's.
 *
 * model.SecretAccessKey and model.SecretSecretKey are reserved for the ACL:
 * they are written only through SetACL and filled from global settings for any
 * profile that named no mechanism. A family storing its own credentials under
 * those names has them cleared on save and RocketMQ's global pair stamped on
 * at dial time, which is a connection that worked in the form and fails
 * everywhere else.
 */
func TestSecretsAvoidTheReservedNames(t *testing.T) {
	reserved := map[string]bool{
		model.SecretAccessKey: true,
		model.SecretSecretKey: true,
	}
	for _, field := range New().Descriptor().Form {
		if field.Target == model.TargetSecret && reserved[field.Key] {
			t.Errorf("secret %q is reserved for the rocketmq acl", field.Key)
		}
	}
}

/*
 * The REST messaging credential does not fall back to the management one.
 *
 * IBM MQ's second credential does fall back, and that is right there: both of
 * its interfaces authenticate against one mqweb user registry, so an account
 * holding both roles is the ordinary deployment. Solace's two are not one
 * registry. A SEMP credential is a management user, broker-wide, with an
 * access level; a REST credential is a client-username, an object inside one
 * Message VPN. Offering "admin" as a client-username would be refused by every
 * broker that checks, and silently accepted by one that does not - which is
 * worse, because it would work until somebody turned authentication on.
 */
func TestRESTCredentialDoesNotBorrowTheManagementOne(t *testing.T) {
	profile := model.ConnectionProfile{
		Endpoints: "http://broker:8080",
		Auth:      model.AuthConfig{Mechanism: model.AuthPlain},
		Secrets: map[string]string{
			SecretUsername: "admin",
			SecretPassword: "admin-password",
		},
	}

	config, err := configOf(profile)
	if err != nil {
		t.Fatalf("configOf: %v", err)
	}
	if !config.rest.empty() {
		t.Errorf("rest credential = %q, want none; a management user is not a client-username",
			config.rest.username)
	}
	if config.admin.username != "admin" {
		t.Errorf("management credential = %q, want admin", config.admin.username)
	}
}

// A profile with no address is refused where the message can say so, rather
// than by the HTTP client failing to parse an empty URL.
func TestConfigRefusesAProfileWithNoAddress(t *testing.T) {
	if _, err := configOf(model.ConnectionProfile{}); err == nil {
		t.Fatal("accepted a profile with no semp address")
	}
}

/*
 * A bare host:port is accepted and gets http, not https.
 *
 * Every other family's endpoint field takes a host and a port, so the muscle
 * memory is real. The default scheme is http because that is what a broker
 * serves SEMP on until it has been given a certificate - the opposite of IBM
 * MQ, whose mqweb server is TLS-only unless reconfigured.
 */
func TestEndpointTakesABareHostAndPort(t *testing.T) {
	for _, test := range []struct{ given, want string }{
		{"127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"http://broker:8080", "http://broker:8080"},
		{"https://broker:943", "https://broker:943"},
		{" broker:8080 , spare:8080", "http://broker:8080"},
	} {
		if got := firstEndpoint(test.given); got != test.want {
			t.Errorf("firstEndpoint(%q) = %q, want %q", test.given, got, test.want)
		}
	}
}

/*
 * The REST messaging port goes on the broker's host, and hostOf is what takes
 * the SEMP port off it.
 *
 * Getting this wrong is not a cosmetic failure: leaving 8080 on would send
 * every message to SEMP, which answers 405 and stores nothing.
 */
func TestHostOfDropsTheSEMPPort(t *testing.T) {
	for _, test := range []struct{ given, want string }{
		{"http://127.0.0.1:8080", "127.0.0.1"},
		{"https://broker.example:943", "broker.example"},
		{"http://broker.example", "broker.example"},
		{"http://broker.example:8080/", "broker.example"},
		{"http://[::1]:8080", "[::1]"},
	} {
		if got := hostOf(test.given); got != test.want {
			t.Errorf("hostOf(%q) = %q, want %q", test.given, got, test.want)
		}
	}
}

/*
 * Object names reach a URL escaped.
 *
 * Solace names are written like topics, so a slash in one is the ordinary case
 * rather than an awkward one. Pasted in raw, "orders/eu" reads as a collection
 * with a sub-resource and SEMP answers NOT_FOUND for a queue that is plainly
 * in the listing.
 */
func TestSegmentEscapesNamesThatLookLikePaths(t *testing.T) {
	for _, test := range []struct{ given, want string }{
		{"orders", "orders"},
		{"mqstudio/seed/orders", "mqstudio%2Fseed%2Forders"},
		{"#DEAD_MSG_QUEUE", "%23DEAD_MSG_QUEUE"},
		{"with space", "with%20space"},
	} {
		if got := segment(test.given); got != test.want {
			t.Errorf("segment(%q) = %q, want %q", test.given, got, test.want)
		}
	}
}

/*
 * A SEMP failure is read out of the envelope, not off the HTTP status.
 *
 * The broker answers a missing object with HTTP 400 and puts NOT_FOUND inside,
 * so a driver keying off the code alone reports every deleted queue as a bad
 * request - and, worse, never recognises the one failure it has to treat as an
 * empty result rather than a broken page.
 */
func TestSEMPErrorsAreReadFromTheEnvelope(t *testing.T) {
	missing := &sempError{HTTPStatus: 400, Status: statusNotFound, Description: "no such queue"}
	if !notFound(missing) {
		t.Error("a NOT_FOUND envelope on http 400 is not recognised as a missing object")
	}
	if notFound(&sempError{HTTPStatus: 400, Status: "NOT_ALLOWED"}) {
		t.Error("a NOT_ALLOWED envelope reads as a missing object")
	}
	if !strings.Contains(missing.Error(), "no such queue") {
		t.Errorf("the error drops the broker's own description: %v", missing)
	}
}

/*
 * The browse caveat is this family's own, and it says the opposite of three of
 * the four it could have been copied from.
 *
 * Every other family that browses pays for it in one way or another: an SQS
 * receive hides what it returns, a Pub/Sub pull delivers it, a Kinesis read
 * spends the shard's allowance every consumer shares. A SEMP browse costs
 * nothing at all - the queue is byte-for-byte the same afterwards - which puts
 * Solace beside Service Bus rather than beside those three. What it cannot do
 * is hand back the message, which is a limit none of the others has.
 *
 * The four keys below are named rather than described because a caveat is a
 * hand-copied string in two places, and the copy that gets made is whichever
 * one the last driver used.
 */
func TestBrowseCaveatIsThisFamilysOwn(t *testing.T) {
	declared := (&Conn{vpn: "default", closed: make(chan struct{})}).declare(tiers{})

	caveat, present := declared.Caveat(model.CapMessageQuery)
	if !present {
		t.Fatal("browsing carries no caveat, and it shows no message body")
	}
	if caveat != browseNoPayload {
		t.Errorf("caveat = %q, want %q", caveat, browseNoPayload)
	}
	for _, borrowed := range []string{
		"mq.sqs.caveat.receiveHides",
		"mq.google-pubsub.caveat.pullDelivers",
		"mq.kinesis.caveat.readQuota",
		"mq.ibmmq.caveat.browseCharacterOnly",
	} {
		if caveat == borrowed {
			t.Errorf("the caveat is %s, which is another family's and says something "+
				"this browse does not do", borrowed)
		}
	}
	if !strings.HasPrefix(caveat, "mq.solace.caveat.") {
		t.Errorf("caveat %q is not in this family's namespace", caveat)
	}
}

/*
 * The REST tier's reasons stay reasons and never become caveats.
 *
 * They are different states with different renderings - a degraded capability
 * is drawn disabled with an explanation, a caveat is drawn normally with a
 * warning - and a driver that put a reason in the wrong map produces a send
 * console that looks usable and is not.
 */
func TestRESTReasonsAreDegradedRatherThanCaveats(t *testing.T) {
	for _, reason := range []string{restUnreachable, restForbidden} {
		declared := (&Conn{vpn: "default", closed: make(chan struct{})}).
			declare(tiers{restReason: reason})
		for _, capability := range restCapabilities() {
			if declared.Has(capability) {
				t.Errorf("%s is still supported with the rest tier reported %s", capability, reason)
			}
			if got, present := declared.DegradedReason(capability); !present || got != reason {
				t.Errorf("%s degraded reason = %q, want %q", capability, got, reason)
			}
			if _, isCaveat := declared.Caveat(capability); isCaveat {
				t.Errorf("%s carries a caveat as well as a reason", capability)
			}
		}
	}
}
