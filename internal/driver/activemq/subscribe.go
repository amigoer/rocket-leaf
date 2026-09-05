package activemq

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Azure/go-amqp"

	"github.com/amigoer/mq-studio/internal/model"
)

// Watching a topic as messages arrive, which is the one thing the management
// plane cannot do: JMX is request/response and has no push.
//
// Topics only, and that is a safety rule rather than a limitation of the
// library. A JMS consumer consumes: attaching one to a queue would take
// messages off it and hand them to a window somebody opened to look. On a
// topic a non-durable subscriber gets a copy of what is published while it
// listens and the queue behind other subscribers is untouched, which is the
// same shape MQTT and NATS have here.
//
// Nothing that arrives is stored. A message delivered while nobody was
// watching is gone, so this is a live view rather than the message board with
// a filter on it - the same distinction the NATS subjects page draws.

// liveBufferDefault is how many messages one stream holds between polls.
//
// A bound on memory, and the reason a stream reports what it dropped: a topic
// nobody is reading can publish faster than a window polls, and silently
// losing the difference would make a busy topic look quiet.
const liveBufferDefault = 500

// liveStream is one running subscription.
type liveStream struct {
	id      string
	filters []model.LiveFilter
	started time.Time

	mu       sync.Mutex
	messages []*model.LiveMessage
	next     int64
	received int64
	dropped  int64
	live     bool

	cancel  context.CancelFunc
	closers []func()
}

// StartLiveSubscription attaches a non-durable receiver to each named topic.
func (c *Conn) StartLiveSubscription(ctx context.Context, spec model.LiveSubscriptionSpec) (*model.LiveSubscription, error) {
	if c.tiers.amqpReason != "" {
		return nil, errors.New(c.tiers.amqpReason)
	}
	if len(spec.Filters) == 0 {
		return nil, errors.New("a live subscription needs at least one topic")
	}

	buffer := spec.Buffer
	if buffer <= 0 {
		buffer = liveBufferDefault
	}

	// Every named destination has to be a topic. Checked before anything is
	// attached, so a spec naming one queue does not leave the other
	// subscriptions running while it reports a failure.
	for _, filter := range spec.Filters {
		detail, err := c.DestinationDetail(ctx, model.DestinationRef{Name: filter.Pattern})
		if err != nil {
			return nil, err
		}
		if detail.Attributes[AttrKind] != string(topicKind) {
			return nil, fmt.Errorf(
				"%q is a queue: attaching a consumer would take its messages, so only topics can be followed",
				filter.Pattern)
		}
	}

	client, err := c.dialAMQPClient(ctx)
	if err != nil {
		return nil, err
	}

	// The stream's own context, not the request's: the request ends when this
	// function returns and the receivers have to outlive it.
	streamCtx, cancel := context.WithCancel(context.Background())
	stream := &liveStream{
		id:      strconv.FormatInt(time.Now().UnixNano(), 36),
		filters: spec.Filters,
		started: time.Now(),
		live:    true,
		cancel:  cancel,
	}
	stream.closers = append(stream.closers, func() { _ = client.Close() })

	session, err := client.NewSession(ctx, nil)
	if err != nil {
		cancel()
		_ = client.Close()
		return nil, err
	}

	for _, filter := range spec.Filters {
		receiver, err := session.NewReceiver(ctx, filter.Pattern, &amqp.ReceiverOptions{
			// Non-durable and unsettled-on-close, so nothing survives this
			// window: a stream that left a durable subscription behind would
			// accumulate a backlog on the broker for a page nobody has open.
			Durability: amqp.DurabilityNone,
			// Credit bounded to the buffer. Without it the broker pushes as
			// fast as it can and the drop count becomes the whole stream.
			Credit: int32(buffer),
		})
		if err != nil {
			cancel()
			stream.closeAll()
			return nil, fmt.Errorf("could not follow %q: %w", filter.Pattern, err)
		}

		pattern := filter.Pattern
		go stream.receive(streamCtx, receiver, pattern, buffer)
	}

	c.streamsMu.Lock()
	if c.streams == nil {
		c.streams = make(map[string]*liveStream, 1)
	}
	c.streams[stream.id] = stream
	c.streamsMu.Unlock()

	return stream.snapshot(), nil
}

// receive pumps one receiver into the stream's buffer until the stream stops.
func (s *liveStream) receive(ctx context.Context, receiver *amqp.Receiver, pattern string, buffer int) {
	s.mu.Lock()
	s.closers = append(s.closers, func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = receiver.Close(closeCtx)
	})
	s.mu.Unlock()

	for {
		message, err := receiver.Receive(ctx, nil)
		if err != nil {
			// The session went down, which is not the same as the topic being
			// quiet - and without this flag the two look identical on screen.
			s.mu.Lock()
			s.live = false
			s.mu.Unlock()
			return
		}
		// Accepted rather than released: a non-durable subscriber on a topic
		// owes the message to nobody else, and leaving it unsettled would hold
		// credit the stream needs for what comes next.
		_ = receiver.AcceptMessage(ctx, message)
		s.append(pattern, message, buffer)
	}
}

func (s *liveStream) append(pattern string, message *amqp.Message, buffer int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.received++
	if len(s.messages) >= buffer {
		// The oldest goes, not the newest: a live view is about what is
		// arriving now, and dropping the arrival would make the page stop
		// updating rather than lose history it never had.
		s.messages = s.messages[1:]
		s.dropped++
	}

	body := ""
	if message.Value != nil {
		body = fmt.Sprint(message.Value)
	} else if len(message.Data) > 0 {
		body = string(message.GetData())
	}

	// The application's own properties, plus the one thing an AMQP receiver
	// knows that the management browse does not: which subject the broker
	// actually routed on, for a message that arrived through a wildcard.
	attributes := map[string]string{AttrProduct: string(artemis)}
	for key, value := range message.ApplicationProperties {
		attributes[key] = fmt.Sprint(value)
	}

	destination := pattern
	if message.Properties != nil && message.Properties.To != nil {
		destination = *message.Properties.To
	}

	s.next++
	s.messages = append(s.messages, &model.LiveMessage{
		Seq:         s.next,
		Destination: destination,
		Filter:      pattern,
		Body:        body,
		ReceivedAt:  time.Now().Format(time.RFC3339),
		Attributes:  attributes,
	})
}

// PollLiveSubscription returns what arrived after a cursor.
func (c *Conn) PollLiveSubscription(_ context.Context, id string, after int64, limit int) (*model.LiveBatch, error) {
	stream, err := c.stream(id)
	if err != nil {
		return nil, err
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()

	batch := &model.LiveBatch{
		Cursor:   stream.next,
		Dropped:  stream.dropped,
		Received: stream.received,
		Live:     stream.live,
	}
	for _, message := range stream.messages {
		if message.Seq <= after {
			continue
		}
		batch.Messages = append(batch.Messages, message)
		if limit > 0 && len(batch.Messages) >= limit {
			// The cursor is the last one handed over rather than the newest
			// held, or a caller taking a limited batch would skip the rest.
			batch.Cursor = message.Seq
			break
		}
	}
	return batch, nil
}

// StopLiveSubscription detaches the receivers and forgets the stream.
func (c *Conn) StopLiveSubscription(_ context.Context, id string) error {
	c.streamsMu.Lock()
	stream, known := c.streams[id]
	delete(c.streams, id)
	c.streamsMu.Unlock()

	if !known {
		return fmt.Errorf("no live subscription %q", id)
	}
	stream.cancel()
	stream.closeAll()
	return nil
}

// LiveSubscriptions is what is running, so a panel that remounts finds its own
// stream again instead of starting a second one.
func (c *Conn) LiveSubscriptions(_ context.Context) ([]*model.LiveSubscription, error) {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()

	running := make([]*model.LiveSubscription, 0, len(c.streams))
	for _, stream := range c.streams {
		running = append(running, stream.snapshot())
	}
	sort.SliceStable(running, func(i, j int) bool { return running[i].ID < running[j].ID })
	return running, nil
}

func (c *Conn) stream(id string) (*liveStream, error) {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()
	stream, known := c.streams[id]
	if !known {
		return nil, fmt.Errorf("no live subscription %q", id)
	}
	return stream, nil
}

// stopLiveStreams is called from Close. The subscriptions live on the broker
// until they are detached, and closing the socket underneath them leaves it
// tearing them down on a timeout rather than on request.
func (c *Conn) stopLiveStreams() {
	c.streamsMu.Lock()
	streams := make([]*liveStream, 0, len(c.streams))
	for _, stream := range c.streams {
		streams = append(streams, stream)
	}
	c.streams = nil
	c.streamsMu.Unlock()

	for _, stream := range streams {
		stream.cancel()
		stream.closeAll()
	}
}

func (s *liveStream) closeAll() {
	s.mu.Lock()
	closers := s.closers
	s.closers = nil
	s.mu.Unlock()
	for _, close := range closers {
		close()
	}
}

func (s *liveStream) snapshot() *model.LiveSubscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &model.LiveSubscription{
		ID:        s.id,
		Filters:   s.filters,
		StartedAt: s.started.Format(time.RFC3339),
		Received:  s.received,
		Dropped:   s.dropped,
		Live:      s.live,
	}
}
