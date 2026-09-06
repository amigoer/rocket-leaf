package bridge

import (
	"context"
	"time"

	sqsdriver "github.com/amigoer/mq-studio/internal/driver/sqs"
	"github.com/amigoer/mq-studio/internal/model"
	sqsservice "github.com/amigoer/mq-studio/internal/service/sqs"
)

// SQSService is the renderer's entry point for the operations only Amazon SQS
// has. Listing and describing queues go through the canonical services; what
// is here is the rest.
type SQSService struct {
	service *sqsservice.Service
}

// NewSQSService wires the bridge to the service.
func NewSQSService(service *sqsservice.Service) *SQSService {
	return &SQSService{service: service}
}

// SQSQueueInput is a queue as the queue form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which an SQS queue has none. What a queue has instead is a set of
// durations the service enforces, and a redrive policy naming another queue.
//
// Every duration is zero when the form left it alone, which on an edit means
// "keep what is stored" rather than "set it to zero": SQS replaces exactly the
// attributes it is given, so an omitted one survives.
type SQSQueueInput struct {
	Name string `json:"name"`
	// FIFO is fixed at creation and has to match the .fifo suffix. It is sent
	// so a mismatch is refused with a message naming the field the user
	// actually filled in, rather than the attribute the service names.
	FIFO bool `json:"fifo"`

	VisibilityTimeoutSec int `json:"visibilityTimeoutSec"`
	DelaySec             int `json:"delaySec"`
	RetentionSec         int `json:"retentionSec"`
	MaxMessageBytes      int `json:"maxMessageBytes"`
	ReceiveWaitSec       int `json:"receiveWaitSec"`

	// DeadLetterQueue is another queue's name; the driver resolves its ARN.
	// Empty leaves the queue without a redrive policy.
	DeadLetterQueue string `json:"deadLetterQueue"`
	MaxReceiveCount int    `json:"maxReceiveCount"`

	// FIFO only. Ignored on a standard queue, where SQS refuses each of them
	// by an attribute name the form never drew.
	ContentBasedDeduplication bool   `json:"contentBasedDeduplication"`
	DeduplicationScope        string `json:"deduplicationScope"`
	FifoThroughputLimit       string `json:"fifoThroughputLimit"`
}

func (input SQSQueueInput) spec() sqsdriver.QueueSpec {
	return sqsdriver.QueueSpec{
		Name:                      input.Name,
		FIFO:                      input.FIFO,
		VisibilityTimeoutSec:      input.VisibilityTimeoutSec,
		DelaySec:                  input.DelaySec,
		RetentionSec:              input.RetentionSec,
		MaxMessageBytes:           input.MaxMessageBytes,
		ReceiveWaitSec:            input.ReceiveWaitSec,
		DeadLetterQueue:           input.DeadLetterQueue,
		MaxReceiveCount:           input.MaxReceiveCount,
		ContentBasedDeduplication: input.ContentBasedDeduplication,
		DeduplicationScope:        input.DeduplicationScope,
		FifoThroughputLimit:       input.FifoThroughputLimit,
	}
}

// CreateQueue declares a queue in the connection's region.
func (s *SQSService) CreateQueue(connID int, input SQSQueueInput) error {
	return s.service.CreateQueue(context.Background(), connID, input.spec())
}

// UpdateQueue changes an existing queue's settings. Whether a queue is FIFO is
// not among them: that is fixed at creation and spelled in its name.
func (s *SQSService) UpdateQueue(connID int, input SQSQueueInput) error {
	return s.service.UpdateQueue(context.Background(), connID, input.spec())
}

// RemoveQueue deletes a queue and everything in it. There is no undo, and SQS
// refuses the name for 60 seconds afterwards.
func (s *SQSService) RemoveQueue(connID int, name string) error {
	return s.service.RemoveQueue(context.Background(), connID, name)
}

// PurgeQueue discards everything the queue holds.
//
// The call returning is not the queue being empty: SQS purges asynchronously,
// may also delete anything sent in the following minute, and may keep
// delivering what was sent before it for about as long.
func (s *SQSService) PurgeQueue(connID int, name string) error {
	return s.service.PurgeQueue(context.Background(), connID, name)
}

// SQSPublishInput is a send as the SQS console collects it.
//
// Deliberately not MessageService.Send's shape. That one takes a topic, tags,
// keys and a delay level - RocketMQ's vocabulary, of which an SQS message has
// the destination and a delay in real seconds. What it cannot carry is the
// table of named attributes an SQS message puts beside its body, or the two
// fields a FIFO queue requires.
type SQSPublishInput struct {
	Queue string `json:"queue"`
	Body  string `json:"body"`
	// Count sends the same body more than once. One when left at zero.
	Count int `json:"count"`
	// DelaySec holds the message back from consumers, up to 900. A FIFO queue
	// takes none: its delay is a queue setting.
	DelaySec int `json:"delaySec"`
	// Attributes are the producer's own, sent as SQS string attributes.
	Attributes map[string]string `json:"attributes"`
	// GroupID is required on a FIFO queue and refused on a standard one.
	GroupID string `json:"groupId"`
	// DeduplicationID is what SQS deduplicates a FIFO message by for five
	// minutes. Blank lets the driver generate one, which is what keeps a
	// repeat of ten from arriving as one message.
	DeduplicationID string `json:"deduplicationId"`
}

// SQSPublishResult is what the send did.
type SQSPublishResult struct {
	Sent int `json:"sent"`
	// MessageID is the first message's. It addresses nothing - no SQS call
	// takes a message id - and is shown so a page can name what it produced.
	MessageID string `json:"messageId"`
}

// Publish sends to one queue and reports how many the service accepted.
func (s *SQSService) Publish(connID int, input SQSPublishInput) (*SQSPublishResult, error) {
	result, err := s.service.Publish(context.Background(), connID, sqsdriver.PublishRequest{
		Queue:           input.Queue,
		Body:            input.Body,
		Count:           input.Count,
		Delay:           time.Duration(input.DelaySec) * time.Second,
		Attributes:      input.Attributes,
		GroupID:         input.GroupID,
		DeduplicationID: input.DeduplicationID,
	})
	if err != nil {
		return nil, err
	}
	return &SQSPublishResult{Sent: result.Sent, MessageID: result.MessageID}, nil
}

// DeadLetterQueues finds the queues other queues redrive into, and which
// queues point at each of them.
//
// A dead-letter queue in SQS is an ordinary queue with a redrive policy aimed
// at it. Nothing marks one, so this is a walk backwards through the topology -
// and a queue every one of whose sources sits outside the connection's queue
// prefix is not found, because the walk starts from what the prefix let
// through.
func (s *SQSService) DeadLetterQueues(connID int) ([]*model.DeadLetterQueue, error) {
	return s.service.DeadLetterQueues(context.Background(), connID)
}
