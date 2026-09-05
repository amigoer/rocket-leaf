// Package activemq drives ActiveMQ Classic and ActiveMQ Artemis over Jolokia,
// the JMX-over-HTTP agent both brokers ship under their web console.
//
// One family, two products. Classic and Artemis are one thing to a user -
// Artemis is where Classic is going, and Amazon MQ offers either behind the
// same console - but they share nothing a driver reads: different agent path,
// different MBean domain, different ObjectName keys, different attribute
// names, and browse results whose map keys do not overlap at all. Which one
// answered is settled when the connection opens, and every read branches on
// it.
//
// The management plane is also the data plane here, which is unusual and is
// the whole reason this driver needs no wire-protocol client for its main
// pages. Browsing and sending are JMX operations on both products - Classic's
// browse() and sendTextMessage(), Artemis's browse(page, size) and
// sendMessage() - and browsing consumes nothing, so the message board carries
// no requeue caveat the way RabbitMQ's does.
//
// AMQP 1.0 is an optional second tier, probed at connect time, for the two
// things JMX cannot do: follow a destination as messages arrive, and send a
// body that is not text. A broker with the acceptor switched off keeps every
// other page and says why those two are missing.
package activemq

import (
	"context"

	"github.com/amigoer/mq-studio/internal/driver"
	"github.com/amigoer/mq-studio/internal/model"
)

// Option and secret keys this driver stores in a connection profile.
//
// A private contract between this package and the connection form in the
// renderer. Another family's "tls" means whatever that family's driver decides
// it means, which is why these are spelled out here rather than shared.
const (
	OptionBrokerName    = "brokerName"
	OptionJolokiaPath   = "jolokiaPath"
	OptionOrigin        = "originHeader"
	OptionAMQPURL       = "amqpUrl"
	OptionTLSSkipVerify = "tlsSkipVerify"

	SecretUsername     = "username"
	SecretPassword     = "password"
	SecretAMQPUsername = "amqpUsername"
	SecretAMQPPassword = "amqpPassword"
)

// defaultPort is the web console's, not a broker port. For this family the
// address a profile carries is HTTP: the console is the management plane, and
// the broker port is the optional extra.
const defaultPort = "8161"

// Driver is the ActiveMQ family, both products.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindActiveMQ }

// Descriptor is the connection form and the family's best-case capabilities.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindActiveMQ,
		DefaultPort:     defaultPort,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				// A console URL rather than a host and port. Written as a
				// list for the same reason every other family's is - the
				// field type is shared - but a second console is a second
				// broker here, so one entry is the ordinary case.
				Key:         "endpoints",
				Target:      model.TargetEndpoints,
				Type:        model.FieldEndpointList,
				LabelKey:    "mq.activemq.form.console",
				Placeholder: "http://127.0.0.1:8161",
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
				// Only two mechanisms, because the console has only two. The
				// broker's own authentication is a JAAS realm configured in
				// XML and has no bearing on reaching Jolokia.
				Key:      "mechanism",
				Target:   model.TargetAuth,
				Type:     model.FieldSelect,
				LabelKey: "mq.activemq.form.mechanism",
				Default:  string(model.AuthPlain),
				Options: []model.FormOption{
					{Value: string(model.AuthPlain), LabelKey: "mq.activemq.form.authPlain"},
					{Value: string(model.AuthNone), LabelKey: "mq.common.form.authNone"},
				},
			},
			{
				Key:      SecretUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.activemq.form.username",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				Key:      SecretPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.activemq.form.password",
				VisibleWhen: &model.FieldCond{
					Field:  "mechanism",
					Equals: []string{string(model.AuthPlain)},
				},
			},
			{
				// Left empty the driver probes both paths and keeps whichever
				// answered, which is also how it learns which product it is
				// talking to. Filling it in is for a deployment behind a
				// reverse proxy that mounts the agent somewhere else.
				Key:         OptionJolokiaPath,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.activemq.form.jolokiaPath",
				Placeholder: "/api/jolokia",
			},
			{
				// The broker name inside every ObjectName. Classic calls
				// itself localhost and Artemis 0.0.0.0 by default, and both
				// are renameable, so the driver reads it off a search rather
				// than assuming - this is the override for a deployment that
				// registers more than one broker in one JVM.
				Key:      OptionBrokerName,
				Target:   model.TargetOption,
				Type:     model.FieldText,
				LabelKey: "mq.activemq.form.brokerName",
			},
			{
				// Not decoration. Both brokers ship jolokia-access.xml with
				// <strict-checking/>, which refuses a request carrying no
				// Origin as coming from the null origin - an HTTP 403 that
				// reads exactly like bad credentials. The driver always sends
				// one; this names it for a deployment whose policy file
				// allows something other than localhost.
				Key:         OptionOrigin,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.activemq.form.origin",
				Placeholder: defaultOrigin,
			},
			{
				Key:      OptionTLSSkipVerify,
				Target:   model.TargetOption,
				Type:     model.FieldSwitch,
				LabelKey: "mq.common.form.tlsSkipVerify",
			},
			{
				// Optional, and its absence is ordinary rather than a
				// misconfiguration: everything except following a destination
				// live and sending a non-text body works without it.
				Key:         OptionAMQPURL,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.activemq.form.amqpUrl",
				Placeholder: "amqp://127.0.0.1:61616",
			},
			{
				// Its own credentials, because the console and the broker
				// authenticate against different realms and a deployment may
				// well use different accounts. Left empty the console's are
				// reused, which is the common case.
				Key:      SecretAMQPUsername,
				Target:   model.TargetSecret,
				Type:     model.FieldText,
				LabelKey: "mq.activemq.form.amqpUsername",
			},
			{
				Key:      SecretAMQPPassword,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.activemq.form.amqpPassword",
			},
		},
	}
}

// Open dials the console, works out which product answered, and probes the
// optional AMQP tier.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
