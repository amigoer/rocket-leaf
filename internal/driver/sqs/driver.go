// Package sqs drives Amazon SQS over the AWS API.
//
// It is the first family here with no broker address. There is nothing to
// dial: a queue is reached by naming a region and signing a request with an
// AWS credential, and the endpoint is derived from the region by the SDK. So
// this driver's connection form declares no endpoint field at all, which is
// what model.DriverDescriptor.RequiresEndpoints reads to let a profile save
// with its Endpoints empty.
//
// The vocabulary maps onto the canonical pages in one place and nowhere else.
// An SQS queue is a destination: it is named, it holds a depth, and a send
// goes to it. There is no second object. SQS has no subscription, consumer
// group or durable reader of any kind - a consumer is whoever calls
// ReceiveMessage, the service keeps no record of who that was, and a
// consumers page here would list an empty set forever. That absence is most
// of what this package's conformance test records.
//
// What it can do with messages is bounded by the same design. ReceiveMessage
// is the only read, and it is not a browse: a received message is hidden for
// its visibility timeout and its receive count goes up, so a consumer running
// at the same time can miss one this app looked at. The driver hands every
// message straight back and still declares the caveat, because handing it back
// is not instantaneous and the receive count does not come back down.
//
// A dead-letter queue is an ordinary queue that another queue's RedrivePolicy
// points at. That is DeadLetterTopology's shape rather than DeadLetterReader's:
// nothing is named after a consumer group, because there are no consumer
// groups.
//
// FIFO queues are covered. A .fifo suffix changes ordering, deduplication and
// what a send must carry - a message group id is mandatory - and none of that
// changes how a queue is listed, created, emptied, deleted or read.
package sqs

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
	// OptionRegion is what an endpoint field would be on any other family.
	// The SDK builds the service endpoint from it, and it is also what the
	// connection row shows in its address column.
	OptionRegion = "region"

	// OptionEndpointURL points the SDK somewhere other than the public
	// regional endpoint: a VPC interface endpoint, or an emulator. It is
	// still signed for OptionRegion, which is why the region stays required.
	OptionEndpointURL = "endpointUrl"

	// OptionQueuePrefix narrows every listing to queues whose name starts
	// with it. An account can hold thousands of queues across every team that
	// uses it, and ListQueues has no other filter.
	OptionQueuePrefix = "queuePrefix"
)

// Secret keys this driver stores in a connection profile.
//
// They are not model.SecretAccessKey and model.SecretSecretKey, and must never
// be: those two are reserved for RocketMQ's ACL, are written only through
// SetACL, and are filled from global settings for any profile that named no
// mechanism. A family reusing them would have its credentials cleared on save
// and RocketMQ's global pair stamped on at dial time.
const (
	SecretAccessKeyID     = "awsAccessKeyId"
	SecretSecretAccessKey = "awsSecretAccessKey"
	SecretSessionToken    = "awsSessionToken"
)

// Driver is the Amazon SQS family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindSQS }

// Descriptor is the connection form and the family's best-case capabilities.
//
// No endpoint field and no default port, both deliberately. There is no
// address a user could type and no port anything listens on: the SDK resolves
// sqs.<region>.amazonaws.com over HTTPS. DriverDescriptor.RequiresEndpoints
// reads the absence of the field, and the connection service then lets the
// profile save with an empty Endpoints.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindSQS,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				Key:         OptionRegion,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.sqs.form.region",
				Placeholder: "eu-west-1",
				Required:    true,
			},
			{
				// Left blank the SDK uses the default credential chain -
				// environment variables, the shared config file, an instance
				// or container role. That is a real way to run this and not a
				// half-filled form, which is why the pair is not required.
				Key:         SecretAccessKeyID,
				Target:      model.TargetSecret,
				Type:        model.FieldText,
				LabelKey:    "mq.sqs.form.accessKeyId",
				Placeholder: "AKIA...",
			},
			{
				Key:      SecretSecretAccessKey,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.sqs.form.secretAccessKey",
			},
			{
				// Only temporary credentials carry one, and they expire: a
				// connection using STS keys stops working when the session
				// does, which is the broker's doing rather than this app's.
				Key:      SecretSessionToken,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.sqs.form.sessionToken",
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
				Key:         OptionQueuePrefix,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.sqs.form.queuePrefix",
				Placeholder: "team-orders-",
			},
			{
				Key:         OptionEndpointURL,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.sqs.form.endpointUrl",
				Placeholder: "https://vpce-0abc.sqs.eu-west-1.vpce.amazonaws.com",
				Validate:    "url",
			},
		},
	}
}

// Open builds the API client and checks the credentials actually reach SQS.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
