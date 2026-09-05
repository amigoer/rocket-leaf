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
