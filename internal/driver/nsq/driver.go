// Package nsq drives an NSQ cluster over the HTTP APIs of nsqd and
// nsqlookupd.
//
// There is no admin protocol to speak. Everything an operator can ask NSQ is
// an HTTP call on the same daemons that carry the messages - /stats on nsqd is
// the whole read side, /topic/* and /channel/* are the whole write side - so
// this driver needs no wire client, the way the ActiveMQ one needs none.
//
// The vocabulary maps onto the canonical pages in two places and nowhere else.
// An NSQ topic is a destination: it is named, it holds a depth, and a publish
// goes to it. An NSQ channel is a subscription: it is the durable
// consumer-group equivalent, every channel under a topic gets a copy of every
// message, and its depth is the backlog. Nothing else in NSQ has a canonical
// counterpart, which is most of what this package's conformance test records.
//
// What it deliberately cannot do is read messages. nsqd hands a message to a
// consumer and stops holding it; there is no log behind the depth to page
// through, no id to look one up by, and no dead-letter destination anywhere.
// A browse page here would open onto nothing that could ever be listed, so
// this driver declares none of the capabilities that draw one.
//
// A cluster is addressed as a set of nsqd HTTP endpoints, and every read fans
// out across all of them: a topic exists on each nsqd that has been asked to
// carry it, and its depth is the sum. nsqlookupd is a second, optional tier -
// the discovery plane, which knows which nsqd holds what but holds nothing
// itself - and it lights up the directory board alone.
//
// The form asks for no credentials because nsqd's HTTP API has none. Its
// --auth-http-address delegates authorisation for clients arriving over the
// TCP protocol and never touches these endpoints, so a username field here
// would be a control that authenticates nothing.
package nsq

import (
	"context"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// Option keys this driver stores in a connection profile.
//
// A private contract between this package and the connection form in the
// renderer, the way every other family's are.
const (
	OptionLookupd       = "lookupdEndpoints"
	OptionTLSSkipVerify = "tlsSkipVerify"
)

// defaultPort is nsqd's HTTP port, not its TCP one. For this family the
// address a profile carries is the management plane; the 4150 a producer
// dials never reaches this driver.
const defaultPort = "4151"

// Driver is the NSQ family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindNSQ }

// Descriptor is the connection form and the family's best-case capabilities.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindNSQ,
		DefaultPort:     defaultPort,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				// Genuinely a list, unlike ActiveMQ's. A topic lives on every
				// nsqd that was asked to carry it and each one answers only
				// for itself, so a cluster is the set of addresses rather
				// than one address that speaks for the rest.
				Key:         "endpoints",
				Target:      model.TargetEndpoints,
				Type:        model.FieldEndpointList,
				LabelKey:    "mq.nsq.form.nsqd",
				Placeholder: "http://127.0.0.1:4151",
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
				// Optional, and its absence is ordinary: a single-node NSQ
				// runs without one. Everything except the directory board
				// works with this empty.
				Key:         OptionLookupd,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.nsq.form.lookupd",
				Placeholder: "http://127.0.0.1:4161",
			},
			{
				Key:      OptionTLSSkipVerify,
				Target:   model.TargetOption,
				Type:     model.FieldSwitch,
				LabelKey: "mq.common.form.tlsSkipVerify",
			},
		},
	}
}

// Open dials every configured address and checks each is the daemon the field
// it was typed into names.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
