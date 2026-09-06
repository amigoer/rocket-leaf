package googlepubsub

import (
	"context"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * TopicSpec is a topic as the topic form collects it.
 *
 * Deliberately not TopicService.Create's shape. That one takes a broker
 * address, a read queue count, a write queue count and a permission string -
 * RocketMQ's vocabulary, of which a Pub/Sub topic has none. What it has
 * instead is almost nothing, and that is worth writing down rather than
 * hiding: everything about delivery belongs to the subscription, so a topic is
 * a name, an optional retention, and labels.
 */
type TopicSpec struct {
	Name string

	// RetentionSec keeps published messages available for a subscription to
	// seek back into. Zero means "leave it alone", which on an edit keeps
	// whatever is stored and on a create takes the service's default - not
	// retained beyond what every subscription still owes.
	RetentionSec int

	// Labels replace the topic's whole set rather than merging into it, which
	// is the API's own behaviour: the update mask names the field, not one
	// key. Nil leaves them alone; an empty map removes them all.
	Labels map[string]string
}

// spec turns the form's fields into the canonical destination spec, which is
// what keeps the attribute keys private to this package.
func (s TopicSpec) spec() model.DestinationSpec {
	attributes := map[string]string{}
	if s.RetentionSec > 0 {
		attributes[AttrRetentionSec] = strconv.Itoa(s.RetentionSec)
	}
	for key, value := range s.Labels {
		if strings.TrimSpace(key) == "" {
			continue
		}
		attributes[AttrLabelPrefix+key] = value
	}
	return model.DestinationSpec{
		Ref:        model.DestinationRef{Name: strings.TrimSpace(s.Name)},
		Attributes: attributes,
	}
}

// CreateTopic declares a topic from a form submission.
func (c *Conn) CreateTopic(ctx context.Context, spec TopicSpec) error {
	return c.CreateDestination(ctx, spec.spec())
}

// UpdateTopic changes an existing topic's settings.
func (c *Conn) UpdateTopic(ctx context.Context, spec TopicSpec) error {
	return c.UpdateDestination(ctx, spec.spec())
}
