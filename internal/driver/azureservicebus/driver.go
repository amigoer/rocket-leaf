// Package azureservicebus drives Azure Service Bus over the Azure SDK.
//
// It is the third hosted family here and the first one that is reached by
// dialling something. SQS names a region and Pub/Sub names a project, and
// neither is an address: the SDK derives one. A Service Bus connection names a
// fully qualified namespace - myns.servicebus.windows.net - which is a DNS
// host that both planes of this driver actually dial, AMQP 1.0 on 5671 and
// HTTPS on 443. So this family declares an endpoint field and
// model.DriverDescriptor.RequiresEndpoints is true, unlike the two hosted
// families before it.
//
// Two clients, because Service Bus is two protocols. azservicebus speaks AMQP
// and is what sends, peeks and reads dead letters; azservicebus/admin speaks
// Atom over HTTPS and is what lists, creates and deletes queues, topics,
// subscriptions and rules. A credential that reaches one reaches the other,
// which is why one connection holds both.
//
// The vocabulary has three objects rather than two. A queue holds messages and
// is read directly; a topic holds none and fans a send out to its
// subscriptions; a subscription holds the copy that was fanned out to it and
// is what a consumer reads. Queues and topics share the destinations board
// because they are the same kind of thing to create, configure and delete -
// what separates them is whether anything else has to exist before a message
// can be read, which the subscriptions board is where to see.
//
// What makes this family different from the two before it is that browsing
// costs nothing. PeekMessages reads without locking and without consuming: a
// peeked message stays exactly where it was, its delivery count does not move,
// and a consumer racing this app misses nothing. SQS's ReceiveMessage and
// Pub/Sub's Pull each had to carry a caveat saying the opposite. So
// CapMessageQuery here carries none, and a test pins that absence - swapping
// peek for receive would be a silent regression in the one thing this family
// does better than its neighbours.
//
// Peek also reaches what no consumer would be offered: a scheduled message
// waiting for its enqueue time, and a deferred one set aside by sequence
// number. Both come back with a state saying which they are.
//
// A dead letter is a first-class sub-entity rather than a topology. Every
// queue and every subscription has a $DeadLetterQueue you open a receiver on
// and read directly - the broker names it, nothing points at it, and there is
// nothing to walk backwards through. That is DeadLetterReader's shape, not
// DeadLetterTopology's.
//
// Subscription rules are the routing topology. A rule is one object deciding
// which of a topic's messages reach one subscription - a SQL filter over the
// message's application properties and system fields, or a correlation filter
// matching them by equality, optionally with an action that rewrites the
// message on the way in. That is a routing decision and it lands on the
// routing page, beside RabbitMQ's exchanges and bindings.
package azureservicebus

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
	// OptionSharedAccessKeyName is which authorization rule on the namespace
	// the key below belongs to. It is a name rather than a credential, so it
	// is an option: RootManageSharedAccessKey is the one every namespace is
	// created with, and a least-privilege deployment has others.
	OptionSharedAccessKeyName = "sharedAccessKeyName"

	// OptionEmulatorManagement is the host:port of the Service Bus emulator's
	// management port, and naming it is what puts this connection in emulator
	// mode.
	//
	// It exists because the emulator is two ports rather than one. The
	// endpoint field carries its AMQP port, which the SDK reaches over plain
	// TCP once the connection string says UseDevelopmentEmulator; the Atom
	// management API is a second port serving plain HTTP, and the admin client
	// composes https://<namespace>/ from the endpoint with no hook of its own.
	// So this driver installs one - see managementTransport in client.go.
	//
	// It is named for the emulator rather than as a general management
	// override because that is the only thing it can be: a real namespace
	// serves both planes on the one host it is named after, so a connection
	// that has to be told a second address is by definition not talking to
	// one. declare() reads that.
	OptionEmulatorManagement = "emulatorManagementHost"

	// OptionEntityPrefix narrows every listing to queues, topics and
	// subscriptions whose name starts with it. A namespace shared between
	// teams holds thousands, and the management API has no filter of its own.
	OptionEntityPrefix = "entityPrefix"
)

// Secret keys this driver stores in a connection profile.
//
// They are not model.SecretAccessKey and model.SecretSecretKey, and must never
// be: those two are reserved for RocketMQ's ACL, are written only through
// SetACL, and are filled from global settings for any profile that named no
// mechanism. A family reusing them would have its credentials cleared on save
// and RocketMQ's global pair stamped on at dial time.
const (
	// SecretSharedAccessKey is the shared access key itself - the base64
	// secret half of the authorization rule named above.
	SecretSharedAccessKey = "azureSharedAccessKey"

	// SecretConnectionString is the whole connection string, pasted in, for
	// the profile that has one rather than its parts.
	//
	// It is a secret and not an option because it contains the key: the string
	// the Azure portal offers is Endpoint=...;SharedAccessKeyName=...;
	// SharedAccessKey=..., and storing it anywhere but the encrypted store
	// would put a namespace credential in connections.json in plain text.
	//
	// When it is set it wins outright, and the endpoint field is still
	// required: a connection has to say which namespace it is for before it is
	// saved, and a string pasted into a form is not something the connection
	// list can read a name out of.
	SecretConnectionString = "azureConnectionString"
)

// Driver is the Azure Service Bus family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindAzureServiceBus }

/*
 * Descriptor is the connection form and the family's best-case capabilities.
 *
 * The endpoint field is the decision worth reading twice, because the two
 * hosted families before this one declare none. A region and a project id are
 * not addresses - the SDK turns each into one - and their forms would have
 * been asking for something the user cannot know. A fully qualified Service
 * Bus namespace is an address: it resolves, and this driver opens an AMQP
 * connection to it on 5671 and sends HTTPS requests to it on 443. Declaring no
 * endpoint here would mean putting a hostname in an option and leaving the
 * connection list's address column blank on a connection that has one.
 *
 * DefaultPort is empty all the same. Both ports are the protocols' own and
 * neither is typed: a namespace is named without one, and the emulator's two
 * are a port on the endpoint and the option beside it.
 */
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindAzureServiceBus,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				// One namespace per connection. It is an endpoint list for
				// the same reason every other family's is - the field type is
				// shared - and a second entry would be a second namespace,
				// which is a second connection.
				Key:         "endpoints",
				Target:      model.TargetEndpoints,
				Type:        model.FieldEndpointList,
				LabelKey:    "mq.azure-servicebus.form.namespace",
				Placeholder: "my-namespace.servicebus.windows.net",
				Required:    true,
			},
			{
				Key:         OptionSharedAccessKeyName,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.azure-servicebus.form.keyName",
				Placeholder: "RootManageSharedAccessKey",
				Default:     "RootManageSharedAccessKey",
			},
			{
				Key:      SecretSharedAccessKey,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.azure-servicebus.form.sharedAccessKey",
			},
			{
				// The whole string, for whoever copied one out of the portal
				// rather than its two halves. Not required, because the pair
				// above is the other way to fill this form.
				Key:         SecretConnectionString,
				Target:      model.TargetSecret,
				Type:        model.FieldPassword,
				LabelKey:    "mq.azure-servicebus.form.connectionString",
				Placeholder: "Endpoint=sb://...;SharedAccessKeyName=...;SharedAccessKey=...",
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
				Key:         OptionEntityPrefix,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.azure-servicebus.form.entityPrefix",
				Placeholder: "team-orders-",
			},
			{
				Key:         OptionEmulatorManagement,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.azure-servicebus.form.emulatorManagement",
				Placeholder: "127.0.0.1:5300",
				Validate:    "host-port",
			},
		},
	}
}

// Open builds both clients and checks the credential actually reaches the
// namespace.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
