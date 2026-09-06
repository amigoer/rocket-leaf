package ibmmq

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Creating and deleting, through whichever of the two interfaces owns the
 * object.
 *
 * A queue is a REST resource, so it is a POST and a DELETE. A topic is not one
 * at any version of the API, so it is DEFINE TOPIC and DELETE TOPIC through
 * MQSC. Both go to the same server with the same credential; which one a call
 * takes is decided by the kind attribute the create form fills, and by a
 * lookup on delete - a name on its own does not say which of the two it is,
 * and asking the wrong endpoint would report the object as missing.
 */

// errNoUpdate is why this driver offers no edit.
//
// Not "not implemented". ALTER changes a live object underneath whatever has
// it open, and the fields worth changing on a queue - its maximum depth, its
// backout queue, whether puts are inhibited - each have their own consequences
// for applications already connected. This driver reads them and does not
// offer one control that writes them all, so CapDestinationUpdate is not
// declared and this method exists only because DestinationAdmin requires it.
var errNoUpdate = errors.New(
	"this driver does not alter a queue or a topic: use MQSC, where each change is its own command")

// CreateDestination declares a queue or a topic.
func (c *Conn) CreateDestination(ctx context.Context, spec model.DestinationSpec) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(spec.Ref.Name)
	if name == "" {
		return errors.New("no name given")
	}
	if err := validObjectName(name); err != nil {
		return err
	}

	if spec.Attributes[AttrKind] == KindTopic {
		return c.createTopic(ctx, name, spec)
	}
	return c.createQueue(ctx, name, spec)
}

// UpdateDestination is not offered. See errNoUpdate.
func (c *Conn) UpdateDestination(_ context.Context, _ model.DestinationSpec) error {
	return errNoUpdate
}

// RemoveDestination deletes a queue or a topic.
//
// A queue holding messages is refused, by the queue manager rather than by
// this driver: DELETE QLOCAL without PURGE fails on a queue with a depth, and
// that check is worth keeping as the default. RemoveQueueGuarded below is how
// a caller asks for the other behaviour.
func (c *Conn) RemoveDestination(ctx context.Context, ref model.DestinationRef) error {
	return c.remove(ctx, ref, false)
}

/*
 * RemoveQueueGuarded is the same delete with the emptiness check made
 * explicit.
 *
 * IBM MQ's default is already guarded - a queue with a depth is refused, and a
 * queue an application has open is refused whatever else is asked - so ifEmpty
 * true is the ordinary delete. ifEmpty false is the caller saying it knows the
 * queue holds messages and wants them gone with it, which is PURGE, and it is
 * the only thing here that discards data.
 *
 * ifUnused is not passed on because it cannot be turned off: the queue manager
 * refuses to delete an object something has open, and there is no flag that
 * overrides it.
 */
func (c *Conn) RemoveQueueGuarded(ctx context.Context, ref model.DestinationRef, _, ifEmpty bool) error {
	return c.remove(ctx, ref, !ifEmpty)
}

func (c *Conn) remove(ctx context.Context, ref model.DestinationRef, purge bool) error {
	if err := c.live(); err != nil {
		return err
	}
	name := strings.TrimSpace(ref.Name)
	if name == "" {
		return errors.New("no name given")
	}

	kind, err := c.kindOf(ctx, name)
	if err != nil {
		return err
	}
	if kind == KindTopic {
		// A topic holds nothing, so there is nothing to purge. Its
		// subscriptions survive it: they resolve against the topic string,
		// which goes on existing under whatever object covers it next.
		return c.command(ctx, "delete", "topic", name, nil)
	}

	path := fmt.Sprintf("/qmgr/%s/queue/%s", c.qmgr, name)
	if purge {
		// A flag rather than a value. The server refuses purge=true outright,
		// which is the sort of thing that reads as a broken driver.
		path += "?purge"
	}
	return c.rest.adminSend(ctx, "DELETE", path, nil)
}

// createQueue posts the queue resource.
func (c *Conn) createQueue(ctx context.Context, name string, spec model.DestinationSpec) error {
	queueType := strings.TrimSpace(spec.Attributes[AttrQueueType])
	if queueType == "" {
		queueType = "local"
	}

	body := map[string]any{"name": name, "type": queueType}
	if description := spec.Attributes[AttrDescription]; description != "" {
		body["general"] = map[string]any{"description": description}
	}
	if maximum := spec.Attributes[AttrMaxDepth]; maximum != "" {
		depth, err := strconv.Atoi(maximum)
		if err != nil || depth <= 0 {
			return fmt.Errorf("maximum depth %q is not a positive number", maximum)
		}
		body["storage"] = map[string]any{"maximumDepth": depth}
	}
	return c.rest.adminSend(ctx, "POST", "/qmgr/"+c.qmgr+"/queue", body)
}

/*
 * createTopic defines the topic object.
 *
 * The topic string is required and is not the object's name. An application
 * publishes to a string; the object is where that string's settings are
 * attached, and two objects covering overlapping strings is normal. Defaulting
 * the string to the name would create an object nobody publishes through.
 */
func (c *Conn) createTopic(ctx context.Context, name string, spec model.DestinationSpec) error {
	topicString := strings.TrimSpace(spec.Attributes[AttrTopicString])
	if topicString == "" {
		return errors.New("a topic needs a topic string: it is what publishers name, " +
			"and it is not the topic object's own name")
	}

	// No REPLACE: this is a create, and DEFINE with REPLACE would silently
	// rewrite a topic somebody else made under the same name.
	parameters := map[string]any{"topicstr": topicString}
	if description := spec.Attributes[AttrDescription]; description != "" {
		parameters["descr"] = description
	}
	return c.command(ctx, "define", "topic", name, parameters)
}

// kindOf says whether a name is a queue or a topic here, which the caller of a
// delete does not know and the two interfaces disagree about.
func (c *Conn) kindOf(ctx context.Context, name string) (string, error) {
	destination, err := c.DestinationDetail(ctx, model.DestinationRef{Name: name})
	if err != nil {
		return "", err
	}
	return destination.Attribute(AttrKind), nil
}

/*
 * validObjectName applies IBM MQ's own rule, here rather than at the queue
 * manager.
 *
 * Not decoration: a name with a space in it is refused by the command server
 * with a syntax error naming a character position, which reads as a driver
 * fault rather than as a name that was never allowed. Forty-eight characters
 * from letters, digits and . _ / % is the rule for every object type this
 * driver creates.
 */
func validObjectName(name string) error {
	if len(name) > 48 {
		return fmt.Errorf("%q is %d characters; an ibm mq object name is at most 48", name, len(name))
	}
	for _, letter := range name {
		switch {
		case letter >= 'A' && letter <= 'Z',
			letter >= 'a' && letter <= 'z',
			letter >= '0' && letter <= '9',
			letter == '.', letter == '_', letter == '/', letter == '%':
		default:
			return fmt.Errorf("%q contains %q; an ibm mq object name takes letters, digits and . _ / %%",
				name, letter)
		}
	}
	return nil
}
