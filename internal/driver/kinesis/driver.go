// Package kinesis drives Amazon Kinesis Data Streams over the AWS API.
//
// The second family here with no broker address, and it is reached the same
// way SQS is: a stream is named in a region and the request is signed with an
// AWS credential, so there is nothing to dial and the connection form declares
// no endpoint field. model.DriverDescriptor.RequiresEndpoints reads that
// absence, which is what lets a profile save with its Endpoints empty.
//
// # Shards are not partitions
//
// This is the one family whose central object the canonical model has no room
// for, and pretending otherwise is the mistake this package is arranged to
// avoid. model.Destination offers Partitions, an int. A shard is not a number:
// it has an id, a hash key range that decides which partition keys land on it,
// a read quota of its own, and - because shards are split and merged rather
// than resized - a parent, sometimes two, and an end. A shard closed by a
// split still exists and still holds its records until retention expires.
//
// So the two halves are carried separately. Destination.Partitions is the open
// shard count, which is a true statement about a stream and the figure a
// listing wants; everything a count cannot express reaches the UI through
// ShardInspector, a port of its own behind model.CapShards. What this driver
// does not do is declare model.CapPartitions: that capability is backed by
// DestinationStats, whose page is built around a partition number, and a shard
// id put through it would lose exactly the fields that make a shard a shard.
//
// # Browsing is not consuming
//
// GetRecords does not remove anything. Records stay until the retention period
// expires and any number of readers can read the same ones, so the caveat SQS
// and Pub/Sub carry - that a browse hides a message from a real consumer - is
// simply untrue here and must not be copied. What is true is that a shard has
// a hard read quota, five GetRecords calls a second and two megabytes a
// second, shared with every classic consumer reading it. A browse spends that
// budget, so it can throttle a running application without taking anything
// away from it, which is a different consequence and gets a caveat of its own.
//
// # There is no consumer position
//
// A classic Kinesis consumer keeps its position in a DynamoDB table the KCL
// owns; the stream never learns of it. An enhanced fan-out consumer is a real
// object registered on the stream - it can be listed, created and removed, so
// it maps onto model.Subscription - but it carries no position and no backlog
// either. That is why the lag capability is degraded with a reason rather than
// filled in: MillisBehindLatest on a GetRecords response is this app's own
// reader lagging, not a consumer's stored one.
package kinesis

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

	// OptionStreamPrefix narrows every listing to streams whose name starts
	// with it. ListStreams has no filter of its own, so unlike SQS's queue
	// prefix this one is applied here rather than by the service.
	OptionStreamPrefix = "streamPrefix"
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

// Driver is the Amazon Kinesis Data Streams family.
type Driver struct{}

// New creates the driver.
func New() *Driver { return &Driver{} }

// Kind identifies the family.
func (d *Driver) Kind() model.MQKind { return model.KindKinesis }

// Descriptor is the connection form and the family's best-case capabilities.
//
// No endpoint field and no default port, both deliberately. There is no
// address a user could type and no port anything listens on: the SDK resolves
// kinesis.<region>.amazonaws.com over HTTPS. DriverDescriptor.RequiresEndpoints
// reads the absence of the field, and the connection service then lets the
// profile save with an empty Endpoints.
func (d *Driver) Descriptor() model.DriverDescriptor {
	return model.DriverDescriptor{
		Kind:            model.KindKinesis,
		MaxCapabilities: capabilities(),
		Form: []model.FormField{
			{
				Key:         OptionRegion,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.kinesis.form.region",
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
				LabelKey:    "mq.kinesis.form.accessKeyId",
				Placeholder: "AKIA...",
			},
			{
				Key:      SecretSecretAccessKey,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.kinesis.form.secretAccessKey",
			},
			{
				// Only temporary credentials carry one, and they expire: a
				// connection using STS keys stops working when the session
				// does, which is the service's doing rather than this app's.
				Key:      SecretSessionToken,
				Target:   model.TargetSecret,
				Type:     model.FieldPassword,
				LabelKey: "mq.kinesis.form.sessionToken",
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
				Key:         OptionStreamPrefix,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.kinesis.form.streamPrefix",
				Placeholder: "team-orders-",
			},
			{
				Key:         OptionEndpointURL,
				Target:      model.TargetOption,
				Type:        model.FieldText,
				LabelKey:    "mq.kinesis.form.endpointUrl",
				Placeholder: "https://vpce-0abc.kinesis.eu-west-1.vpce.amazonaws.com",
				Validate:    "url",
			},
		},
	}
}

// Open builds the API client and checks the credentials actually reach Kinesis.
func (d *Driver) Open(ctx context.Context, profile model.ConnectionProfile) (driver.Conn, error) {
	return open(ctx, profile)
}
