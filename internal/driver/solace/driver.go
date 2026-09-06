// Package solace drives Solace PubSub+ over SEMP v2, and over no wire client
// at all.
//
// The obvious client for this family is Solace's own C SDK behind cgo, which
// would put a native library on the critical path of every build in this
// repository. What is here instead is the shape ActiveMQ and IBM MQ already
// use: the management plane is HTTP with JSON, so the driver is the standard
// library and nothing else. SEMP v2 is two halves on one port:
//
//   - /SEMP/v2/config creates, changes and deletes objects - Message VPNs,
//     queues, topic endpoints, the topic subscriptions on them.
//   - /SEMP/v2/monitor reports their state and counts, including the messages
//     spooled on a queue.
//
// # There is an address, and it is the broker's
//
// It would be easy to file Solace with the hosted families on the strength of
// Solace Cloud. It is not one of them. The driver dials http://host:8080 - a
// DNS host and a TCP port a user types - and everything else is a path on it,
// so the form declares a required endpoint field and RequiresEndpoints reads
// that. What the driver never opens is 55555: the SMF listener belongs to
// applications, not to this app.
//
// # A Message VPN is a scope, not part of the address
//
// One broker hosts many Message VPNs and every object this driver reads lives
// inside one, so a connection names one. That makes it a scope rather than an
// address, and the contrast with IBM MQ's queue manager is exact rather than a
// matter of taste. A queue manager is a separate process with its own storage,
// its own log and its own listener, and internal/driver/ibmmq/driver.go rules
// it out of model.CapConnectionScope on those grounds. A Message VPN is none
// of them: it is a partition inside one running broker, sharing that broker's
// process, its message spool, its disk and its listeners. The broker answers
// for itself with no VPN named - its version, its spool, its rates - and it
// enumerates its VPNs on request, which is what ScopeInspector.ListScopes
// returns. Switching one re-points every board at once and dials nothing new,
// which is the whole of what the shell's scope switcher is for.
//
// # Two ports, two protocols
//
// SEMP manages and does not carry messages. Publishing goes through the REST
// messaging interface, which is a different port on the same broker - 9000 by
// default, and configured per Message VPN rather than per broker. So the
// driver probes it when the connection opens and degrades the send console
// with a reason when it does not answer, the way IBM MQ's messaging interface
// and ActiveMQ's AMQP acceptor are. The credential is its own: a SEMP
// management user and a VPN's client-username are different objects in
// different directories, so neither stands in for the other.
package solace

import (
	"context"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// Option keys this driver stores in a connection profile.
//
// A private contract between this package and the connection form in the
// renderer. Another family's "tlsSkipVerify" means whatever that family's
// driver decides it means, which is why these are spelled out here rather
// than shared.
const (
	// OptionMsgVPN names which Message VPN this connection is pointed at, and
	// it is also the descriptor's ScopeOption - the field the shell's scope
	// switcher writes. Left empty the driver takes the broker's only VPN, or
	// "default" when it has one.
	OptionMsgVPN = "msgVpn"

	// OptionRESTURL is the REST messaging interface, for a deployment where it
	// is not simply another port on the SEMP host - behind an ingress, or
	// published on a different address. Left empty the driver derives it from
	// the SEMP host and the port the Message VPN says it listens on.
	OptionRESTURL = "restUrl"

	// OptionTLSSkipVerify turns off certificate verification, and it defaults
	// to off. A broker that has not been given a certificate presents one it
	// generated for itself, so a first connection to a developer installation
	// fails verification - which is a true statement about that installation
	// and must stay a decision the user makes rather than one this driver
	// makes for them.
	OptionTLSSkipVerify = "tlsSkipVerify"
)

// Secret keys this driver stores in a connection profile.
//
// They are not model.SecretAccessKey and model.SecretSecretKey, and must never
// be: those two are reserved for RocketMQ's ACL, are written only through
// SetACL, and are filled from global settings for any profile that named no
// mechanism. A family reusing them would have its own credentials cleared on
// save and RocketMQ's global pair stamped on at dial time.
const (
	SecretUsername = "username"
	SecretPassword = "password"

	// The REST messaging interface's own credentials, and they are not the
	// management ones with a different name. A SEMP credential is a management
	// user, which is broker-wide and has an access level; a REST credential is
	// a client-username, which is an object inside one Message VPN. Reusing
	// one as the other would be wrong far more often than right, so an empty
	// pair here means "send without one" - which is what a VPN whose basic
	// authentication type is none expects.
	SecretRESTUsername = "restUsername"
	SecretRESTPassword = "restPassword"
)

// defaultPort is SEMP's, not SMF's. For this family the address a profile
// carries is the management plane; 55555 is where applications go.
const defaultPort = "8080"

// Driver is the Solace PubSub+ family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindSolace }

// Descriptor is the connection form and the family's best-case capabilities.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindSolace,
		DefaultPort:     defaultPort,
		MaxCapabilities: capabilities(),
		ScopeOption:     OptionMsgVPN,
		Form: []model.FormField{
			{
				// The broker's SEMP URL rather than a host and port. Written
				// as a list because the field type is shared with every other
				// family; a second broker is a second connection here, so one
				// entry is the ordinary case.
				Key:         "endpoints",
				Target:      model.TargetEndpoints,
				Type:        model.FieldEndpointList,
				LabelKey:    "mq.solace.form.semp",
				Placeholder: "http://127.0.0.1:8080",
				Required:    true,
				Validate:    "url",
			},
			{
				// Left empty the driver asks the broker which Message VPNs it
				// hosts and takes the answer when there is one, or "default"
				// when that exists. Filling it in is for a broker hosting
				// several, and for making a profile say out loud which one it
				// is for. It is the same field the scope switcher writes.
				Key:         OptionMsgVPN,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.solace.form.msgVpn",
				Placeholder: "default",
			},
			{
				Key:      "timeoutSec",
				Target:   model.TargetOption,
				Type:     model.FieldNumber,
				LabelKey: "mq.common.form.timeoutSec",
				Default:  "10",
				Validate: "int-range",
			},
			{
				Key:      "mechanism",
				Target:   model.TargetAuth,
				Type:     model.FieldSelect,
				LabelKey: "mq.solace.form.mechanism",
				Default:  string(model.AuthPlain),
				Options: []model.FormOption{
					{Value: string(model.AuthPlain), LabelKey: "mq.solace.form.authPlain"},
					{Value: string(model.AuthNone), LabelKey: "mq.common.form.authNone"},
				},
			},
			{
				Key:      SecretUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.solace.form.username",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				Key:      SecretPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.solace.form.password",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				Key:      OptionTLSSkipVerify,
				Target:   model.TargetOption,
				Type:     model.FieldSwitch,
				LabelKey: "mq.common.form.tlsSkipVerify",
			},
			{
				// Empty is the ordinary case: the driver reads the port the
				// Message VPN listens on and puts it on the SEMP host.
				Key:         OptionRESTURL,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.solace.form.restUrl",
				Placeholder: "http://127.0.0.1:9000",
			},
			{
				Key:      SecretRESTUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.solace.form.restUsername",
			},
			{
				Key:      SecretRESTPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.solace.form.restPassword",
			},
		},
	}
}

// Open dials the broker, settles which Message VPN the profile meant, and
// probes the REST messaging tier.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
