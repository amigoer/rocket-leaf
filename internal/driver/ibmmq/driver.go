// Package ibmmq drives IBM MQ over the two HTTP interfaces the mqweb server
// hosts, and over no wire client at all.
//
// The obvious client for this family is cgo over IBM's native MQ libraries,
// which would mean every build of this app - and every CI job - needed the MQ
// redistributable client installed before it compiled. What is here instead
// is the shape ActiveMQ already uses: the management plane is HTTP, so the
// driver is the standard library and a few hundred lines. Two interfaces,
// both under https://host:9443:
//
//   - /ibmmq/rest/v1/admin describes the queue manager's objects as JSON and
//     runs MQSC for the ones it has no resource for.
//   - /ibmmq/rest/v1/messaging carries one message at a time, with the message
//     descriptor in HTTP headers and the body as the HTTP body.
//
// # There is an address, and it is the mqweb server's
//
// IBM MQ names a host, a port, a channel and a queue manager, and it would be
// easy to file this family with the hosted ones on the strength of the last
// two. It is not one of them. The driver dials https://host:9443 - a DNS host
// and a TCP port a user types - and everything else is a path on it, so the
// form declares a required endpoint field and RequiresEndpoints reads that.
// What the driver never opens is 1414: the queue manager's own listener and
// its server-connection channel belong to applications, not to this app.
//
// # The queue manager is part of the address, not a scope
//
// One mqweb server can front several queue managers, and this driver still
// speaks to exactly one - named on the form, or discovered when the server
// fronts a single one. It is not model.CapConnectionScope: that capability is
// for a naming convention a client puts on resource names, where an unscoped
// connection is the ordinary case and a name nothing carries yet is still
// usable. A queue manager is none of those things. It is a separate process
// with its own storage, its own log, its own listener and its own objects,
// nothing crosses between two of them, and there is no unscoped IBM MQ
// connection at all. A second queue manager is a second connection.
//
// # Two interfaces, two authorisations
//
// The mqweb server maps the administrative interface to the MQWebAdmin role
// and the messaging interface to MQWebUser, and a deployment is free to give
// an account one and not the other - IBM's own developer image does exactly
// that, with one user per role. So the form collects a second, optional
// credential for the messaging interface, and the connection probes that
// interface when it opens. A credential holding only the administrative role
// keeps every board except the two that touch messages, and those two say why
// they are unavailable rather than disappearing.
//
// # Channels
//
// A channel is the family's own first-class object and the shared vocabulary
// has nothing shaped like one, so it gets a port and a capability of its own.
// The argument is in internal/model/channel.go.
package ibmmq

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
	// OptionQueueManager names which queue manager behind the mqweb server
	// this connection is for. Left empty the driver discovers it, which works
	// whenever the server fronts exactly one.
	OptionQueueManager = "queueManager"

	// OptionTLSSkipVerify turns off certificate verification, and it defaults
	// to off. The mqweb server generates a self-signed certificate for itself
	// unless it has been given a real one, so a first connection to a
	// developer installation fails verification - which is a true statement
	// about that installation and must stay a decision the user makes rather
	// than one this driver makes for them.
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

	// The messaging interface's own credentials. Left empty the
	// administrative ones are reused, which is the common case: a deployment
	// that maps both roles to one group has one account for both.
	SecretMessagingUsername = "messagingUsername"
	SecretMessagingPassword = "messagingPassword"
)

// defaultPort is the mqweb server's, not the queue manager's. For this family
// the address a profile carries is HTTPS: the web server is the management
// plane, and 1414 is where applications go.
const defaultPort = "9443"

// Driver is the IBM MQ family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindIBMMQ }

// Descriptor is the connection form and the family's best-case capabilities.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindIBMMQ,
		DefaultPort:     defaultPort,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				// The mqweb server's URL rather than a host and port. Written
				// as a list because the field type is shared with every other
				// family; a second server is a second installation here, so
				// one entry is the ordinary case.
				Key:         "endpoints",
				Target:      model.TargetEndpoints,
				Type:        model.FieldEndpointList,
				LabelKey:    "mq.ibmmq.form.mqweb",
				Placeholder: "https://127.0.0.1:9443",
				Required:    true,
				Validate:    "url",
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
				// Two mechanisms, because the server has two. The queue
				// manager's own authentication is a CONNAUTH object applied to
				// its channels and has no bearing on reaching mqweb.
				Key:      "mechanism",
				Target:   model.TargetAuth,
				Type:     model.FieldSelect,
				LabelKey: "mq.ibmmq.form.mechanism",
				Default:  string(model.AuthPlain),
				Options: []model.FormOption{
					{Value: string(model.AuthPlain), LabelKey: "mq.ibmmq.form.authPlain"},
					{Value: string(model.AuthNone), LabelKey: "mq.common.form.authNone"},
				},
			},
			{
				Key:      SecretUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.ibmmq.form.username",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				Key:      SecretPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.ibmmq.form.password",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				// Left empty the driver asks the server which queue managers
				// it fronts and takes the answer when there is exactly one.
				// Filling it in is for a server fronting several, and for
				// making a profile say out loud which one it is for.
				Key:         OptionQueueManager,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.ibmmq.form.queueManager",
				Placeholder: "QM1",
			},
			{
				Key:      OptionTLSSkipVerify,
				Target:   model.TargetOption,
				Type:     model.FieldSwitch,
				LabelKey: "mq.common.form.tlsSkipVerify",
			},
			{
				// Its own credentials, because the two interfaces authorise
				// against two roles and a deployment may well hold them on
				// different accounts. Left empty the administrative ones are
				// reused, which is the common case.
				Key:      SecretMessagingUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.ibmmq.form.messagingUsername",
			},
			{
				Key:      SecretMessagingPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.ibmmq.form.messagingPassword",
			},
		},
	}
}

// Open dials the mqweb server, settles which queue manager the profile meant,
// and probes the messaging interface.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
