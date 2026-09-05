package bridge

import (
	"context"

	nsqservice "github.com/amigoer/mq-studio/internal/service/nsq"
)

// NSQService is the renderer's entry point for the operations only NSQ has.
// Topics, channels and publishing all go through the canonical services; what
// is here is the rest.
type NSQService struct {
	service *nsqservice.Service
}

// NewNSQService wires the bridge to the service.
func NewNSQService(service *nsqservice.Service) *NSQService {
	return &NSQService{service: service}
}

// CreateTopic declares a topic on every nsqd in the connection.
//
// Not TopicService.Create, whose input is a broker address, two queue counts
// and a permission string - RocketMQ's vocabulary, of which an NSQ topic has
// none. A name is the whole of what one is.
func (s *NSQService) CreateTopic(connID int, name string) error {
	return s.service.CreateTopic(context.Background(), connID, name)
}

// RemoveTopic deletes a topic, its channels and its registration in the
// discovery tier. There is no undo.
func (s *NSQService) RemoveTopic(connID int, name string) error {
	return s.service.RemoveTopic(context.Background(), connID, name)
}

// EmptyTopic discards what the topic and every channel under it are holding.
func (s *NSQService) EmptyTopic(connID int, name string) error {
	return s.service.EmptyTopic(context.Background(), connID, name)
}

// SetTopicPaused stops or resumes delivery into a topic's channels.
//
// A boolean rather than two methods, because it is one control with two
// positions and the page reads the current state back off the topic listing.
func (s *NSQService) SetTopicPaused(connID int, name string, paused bool) error {
	return s.service.SetTopicPaused(context.Background(), connID, name, paused)
}

// NSQChannelInput names a channel, which takes two fields because a channel
// belongs to a topic: "analytics" under two topics is two channels with
// nothing in common.
type NSQChannelInput struct {
	Topic   string `json:"topic"`
	Channel string `json:"channel"`
}

// CreateChannel declares a channel on every nsqd carrying its topic.
//
// There is no position to start it from. What it gets is whatever nsqd is
// still holding: nothing, on a topic that already has a channel, and the
// topic's own queue on one that had none to copy into.
func (s *NSQService) CreateChannel(connID int, input NSQChannelInput) error {
	return s.service.CreateChannel(context.Background(), connID, input.Topic, input.Channel)
}

// RemoveChannel deletes a channel and its backlog, in the discovery tier as
// well as on every nsqd. There is no undo.
func (s *NSQService) RemoveChannel(connID int, input NSQChannelInput) error {
	return s.service.RemoveChannel(context.Background(), connID, input.Topic, input.Channel)
}

// EmptyChannel discards one channel's backlog and leaves the rest of the topic
// alone, which is what separates it from emptying the topic.
func (s *NSQService) EmptyChannel(connID int, input NSQChannelInput) error {
	return s.service.EmptyChannel(context.Background(), connID, input.Topic, input.Channel)
}

// SetChannelPaused stops or resumes delivery to one channel's consumers. The
// other channels under the topic keep running.
func (s *NSQService) SetChannelPaused(connID int, input NSQChannelInput, paused bool) error {
	return s.service.SetChannelPaused(
		context.Background(), connID, input.Topic, input.Channel, paused)
}
