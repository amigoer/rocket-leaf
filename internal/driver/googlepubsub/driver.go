// Package googlepubsub drives Google Cloud Pub/Sub over the Pub/Sub API.
//
// It is the second family here with no broker address, and the first with two
// objects rather than one. There is nothing to dial: a topic is reached by
// naming a project and signing a request with a Google credential, and the
// endpoint is pubsub.googleapis.com for every project there is. So this
// driver's connection form declares no endpoint field at all, which is what
// model.DriverDescriptor.RequiresEndpoints reads to let a profile save with
// its Endpoints empty.
//
// The vocabulary is where this family differs from the one before it. SQS has
// a queue and nothing else; Pub/Sub has a topic and a subscription, and the
// subscription is a real object - created, listed and deleted on its own,
// outliving nothing and nobody. A topic holds no messages: it fans a publish
// out to whatever subscriptions exist at that moment and discards it if none
// do. That single fact is what most of this package is arranged around, and
// what makes a topic with no subscription the fault worth alerting on.
//
// What it can do with messages is bounded by there being only one read. Pull
// is the whole of it, and it is not a browse: a pulled message is held for the
// subscription's ack deadline, so a real consumer racing this app can miss
// one, and its delivery attempt counts towards the dead-letter policy's limit.
// The driver hands every message straight back and still declares the caveat,
// because handing it back is not instantaneous and the attempt does not come
// back down.
//
// A dead-letter topic is an ordinary topic that a subscription's
// DeadLetterPolicy points at. That is DeadLetterTopology's shape rather than
// DeadLetterReader's - one object points at another - and the policy sits on
// the subscription, which is why each source names both the topic it came from
// and the subscription that gave up on it.
package googlepubsub

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
	// OptionProjectID is what an endpoint field would be on any other family.
	// Every resource name begins with it, and it is also what the connection
	// row shows in its address column.
	OptionProjectID = "projectId"

	// OptionEmulatorHost is the host:port of a Pub/Sub emulator, which is the
	// same value the client library's own PUBSUB_EMULATOR_HOST carries. It is
	// named for the emulator rather than as a general endpoint override
	// because that is the only thing it can be: the real service has one
	// address for every project, so a connection that names a host is by
	// definition not talking to it. declare() reads that.
	OptionEmulatorHost = "emulatorHost"

	// OptionResourcePrefix narrows every listing to topics and subscriptions
	// whose short name starts with it. A project can hold thousands across
	// every team that uses it, and the API has no filter of its own.
	OptionResourcePrefix = "resourcePrefix"
)

// Secret keys this driver stores in a connection profile.
//
// They are not model.SecretAccessKey and model.SecretSecretKey, and must never
// be: those two are reserved for RocketMQ's ACL, are written only through
// SetACL, and are filled from global settings for any profile that named no
// mechanism. A family reusing them would have its credentials cleared on save
// and RocketMQ's global pair stamped on at dial time.
const (
	// SecretCredentialsJSON holds the service account key itself, pasted in,
	// rather than a path to the file it came in.
	//
	// A path would be the smaller field and the worse one. It points at
	// something this app does not own: the key stays in plain text wherever it
	// was downloaded, the profile stops working the moment the file is moved,
	// and a profile exported to another machine takes a broken pointer with
	// it. Pasted, the key goes through the same encrypted store as every other
	// secret here and travels with the profile. The case a path exists for -
	// "the machine already holds the identity" - is Application Default
	// Credentials, which this driver uses whenever the field is left blank and
	// which reads GOOGLE_APPLICATION_CREDENTIALS among other things.
	SecretCredentialsJSON = "googleCredentialsJson"
)

// Driver is the Google Cloud Pub/Sub family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindGooglePubSub }

// Descriptor is the connection form and the family's best-case capabilities.
//
// No endpoint field and no default port, both deliberately. There is no
// address a user could type and no port anything listens on: the client
// resolves pubsub.googleapis.com over HTTPS for every project.
// DriverDescriptor.RequiresEndpoints reads the absence of the field, and the
// connection service then lets the profile save with an empty Endpoints.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindGooglePubSub,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				Key:         OptionProjectID,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.google-pubsub.form.projectId",
				Placeholder: "my-project",
				Required:    true,
			},
			{
				// Left blank the client uses Application Default Credentials -
				// GOOGLE_APPLICATION_CREDENTIALS, a gcloud user login, the
				// metadata server on a GCE or GKE workload. That is a real way
				// to run this and not a half-filled form, which is why the key
				// is not required.
				Key:         SecretCredentialsJSON,
				Target:      model.TargetSecret,
				Type:        model.FieldPassword,
				LabelKey:    "mq.google-pubsub.form.credentialsJson",
				Placeholder: `{"type":"service_account", ...}`,
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
				Key:         OptionResourcePrefix,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.google-pubsub.form.resourcePrefix",
				Placeholder: "team-orders-",
			},
			{
				Key:         OptionEmulatorHost,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.google-pubsub.form.emulatorHost",
				Placeholder: "127.0.0.1:8681",
			},
		},
	}
}

// Open builds the API client and checks the credential actually reaches the
// project.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
