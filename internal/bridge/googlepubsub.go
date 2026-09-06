package bridge

import (
	"context"

	pubsubdriver "github.com/amigoer/mq-studio/internal/driver/googlepubsub"
	pubsubservice "github.com/amigoer/mq-studio/internal/service/googlepubsub"
)

// GooglePubSubService is the renderer's entry point for the operations only
// Google Pub/Sub has. Listing and describing topics go through the canonical
// services; what is here is the rest.
type GooglePubSubService struct {
	service *pubsubservice.Service
}

// NewGooglePubSubService wires the bridge to the service.
func NewGooglePubSubService(service *pubsubservice.Service) *GooglePubSubService {
	return &GooglePubSubService{service: service}
}

// GooglePubSubTopicInput is a topic as the topic form collects it.
//
// Deliberately not TopicService.Create's shape. That one takes a broker
// address, two queue counts and a permission string - RocketMQ's vocabulary,
// of which a Pub/Sub topic has none. What a topic has instead is almost
// nothing, because everything about delivery belongs to the subscription.
type GooglePubSubTopicInput struct {
	Name string `json:"name"`

	// RetentionSec keeps published messages available for a subscription to
	// seek back into. Zero leaves it alone, which on an edit means "keep what
	// is stored" and on a create takes the service's default.
	RetentionSec int `json:"retentionSec"`

	// Labels replace the topic's whole set rather than merging into it, which
	// is the API's own behaviour: the update mask names the field, not one key.
	Labels map[string]string `json:"labels"`
}

func (input GooglePubSubTopicInput) spec() pubsubdriver.TopicSpec {
	return pubsubdriver.TopicSpec{
		Name:         input.Name,
		RetentionSec: input.RetentionSec,
		Labels:       input.Labels,
	}
}

// CreateTopic declares a topic in the connection's project.
func (s *GooglePubSubService) CreateTopic(connID int, input GooglePubSubTopicInput) error {
	return s.service.CreateTopic(context.Background(), connID, input.spec())
}

// UpdateTopic changes an existing topic's settings. Labels are replaced
// wholesale, so a form editing them sends the complete set.
func (s *GooglePubSubService) UpdateTopic(connID int, input GooglePubSubTopicInput) error {
	return s.service.UpdateTopic(context.Background(), connID, input.spec())
}

// RemoveTopic deletes a topic. Its subscriptions survive it, pointing at
// _deleted-topic_ and unable to receive anything ever again.
func (s *GooglePubSubService) RemoveTopic(connID int, name string) error {
	return s.service.RemoveTopic(context.Background(), connID, name)
}
