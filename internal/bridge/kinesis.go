package bridge

import (
	"context"

	kinesisdriver "github.com/amigoer/mq-studio/internal/driver/kinesis"
	"github.com/amigoer/mq-studio/internal/model"
	kinesisservice "github.com/amigoer/mq-studio/internal/service/kinesis"
)

// KinesisService is the renderer's entry point for the operations only Amazon
// Kinesis has. Listing and describing streams go through the canonical
// services; what is here is the rest.
type KinesisService struct {
	service *kinesisservice.Service
}

// NewKinesisService wires the bridge to the service.
func NewKinesisService(service *kinesisservice.Service) *KinesisService {
	return &KinesisService{service: service}
}

// KinesisStreamInput is a stream as the stream form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Kinesis stream has none.
type KinesisStreamInput struct {
	Name string `json:"name"`
	// OnDemand hands the capacity to AWS. It is the one field that decides
	// whether Shards means anything: CreateStream refuses a shard count beside
	// an on-demand mode, and UpdateShardCount refuses an on-demand stream.
	OnDemand bool `json:"onDemand"`
	// Shards is the target for a provisioned stream. On an edit the service
	// reaches it by splitting and merging, which leaves the old shards closed
	// and still holding their records.
	Shards int `json:"shards"`
	// RetentionHours is between 24 and 8760. Zero on an edit means "keep what
	// is stored", which is what lets a resize leave the retention alone.
	RetentionHours int `json:"retentionHours"`
}

func (input KinesisStreamInput) spec() kinesisdriver.StreamSpec {
	return kinesisdriver.StreamSpec{
		Name:           input.Name,
		OnDemand:       input.OnDemand,
		Shards:         input.Shards,
		RetentionHours: input.RetentionHours,
	}
}

// CreateStream declares a stream in the connection's region.
func (s *KinesisService) CreateStream(connID int, input KinesisStreamInput) error {
	return s.service.CreateStream(context.Background(), connID, input.spec())
}

// UpdateStream changes an existing stream's capacity mode, shard count or
// retention.
//
// Not one call: each of the three is its own asynchronous operation and each
// is refused while the stream is settling from the last, so this returns when
// the last of them has been accepted rather than when the stream is done.
func (s *KinesisService) UpdateStream(connID int, input KinesisStreamInput) error {
	return s.service.UpdateStream(context.Background(), connID, input.spec())
}

// RemoveStream deletes a stream and every record in it.
//
// A stream with registered consumers is refused rather than cascaded: an
// enhanced fan-out consumer is an application somebody stood up, and removing
// it as a side effect would be a bigger action than the one asked for.
func (s *KinesisService) RemoveStream(connID int, name string) error {
	return s.service.RemoveStream(context.Background(), connID, name)
}

// Shards lists a stream's shards, open and closed.
//
// Closed ones are included on purpose. A shard split or merged yesterday takes
// no more writes and still holds every record written to it until retention
// expires, so leaving it out would hide both those records and the reason the
// stream reports fewer open shards than the page lists rows.
func (s *KinesisService) Shards(connID int, stream string) ([]*model.Shard, error) {
	return s.service.Shards(context.Background(), connID, stream)
}

// KinesisPublishInput is a send as the Kinesis console collects it.
//
// Deliberately not MessageService.Send's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary, of which a Kinesis record
// has the destination and the partition key. There is no header table, no tag
// and nothing anywhere in the service that holds a record back.
type KinesisPublishInput struct {
	Stream string `json:"stream"`
	Body   string `json:"body"`
	// PartitionKey is required. Kinesis hashes it into the 128-bit key space
	// and the shard whose range covers the hash takes the record, so it is
	// what decides where a record lands and the only ordering guarantee the
	// service makes is between records sharing one.
	PartitionKey string `json:"partitionKey"`
	// ExplicitHashKey overrides that hash with one the sender chooses, which
	// is the only way to aim a record at a named shard. Blank is the ordinary
	// case.
	ExplicitHashKey string `json:"explicitHashKey"`
	// Count sends the same body more than once. One when left at zero. Each
	// copy gets its own partition key suffix unless a hash key aims them all
	// at one shard.
	Count int `json:"count"`
}

// KinesisPublishResult is what the send did.
type KinesisPublishResult struct {
	Sent int `json:"sent"`
	// SequenceNumber and ShardID are the first record's, and both are given
	// because neither addresses a record on its own.
	SequenceNumber string `json:"sequenceNumber"`
	ShardID        string `json:"shardId"`
	// Failed counts records the service refused individually. A send that
	// raised no error can still have refused most of its batch.
	Failed int `json:"failed"`
}

// Publish sends to one stream and reports what the service took.
func (s *KinesisService) Publish(connID int, input KinesisPublishInput) (*KinesisPublishResult, error) {
	result, err := s.service.Publish(context.Background(), connID, kinesisdriver.PublishRequest{
		Stream:          input.Stream,
		Body:            input.Body,
		PartitionKey:    input.PartitionKey,
		ExplicitHashKey: input.ExplicitHashKey,
		Count:           input.Count,
	})
	if err != nil {
		return nil, err
	}
	return &KinesisPublishResult{
		Sent:           result.Sent,
		SequenceNumber: result.SequenceNumber,
		ShardID:        result.ShardID,
		Failed:         result.Failed,
	}, nil
}

// RegisterConsumer registers an enhanced fan-out consumer on a stream.
//
// Registering is not subscribing. It reserves the name and the dedicated two
// megabytes a second of read throughput this consumer gets on every shard;
// an application then subscribes against the ARN. Nothing here reads on its
// behalf, and the registration outlives whatever does.
func (s *KinesisService) RegisterConsumer(connID int, stream, name string) error {
	return s.service.RegisterConsumer(context.Background(), connID, stream, name)
}

// DeregisterConsumer removes a registration.
//
// The stream is required as well as the name: a consumer name is unique only
// within its stream, and every call that names one takes the stream too.
func (s *KinesisService) DeregisterConsumer(connID int, stream, name string) error {
	return s.service.DeregisterConsumer(context.Background(), connID, stream, name)
}
