package azureservicebus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus/admin"
	"golang.org/x/sync/errgroup"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Rules, which are this family's routing topology.
 *
 * Every other family here decides what a reader gets with a field on the
 * reader: a Pub/Sub subscription carries a filter string, an SQS queue carries
 * nothing at all. A Service Bus subscription carries *rules* - separate
 * objects with names, created and deleted on their own, several to a
 * subscription - and each one is a filter plus an optional action that
 * rewrites the message on the way in. Which messages reach which subscription
 * is therefore a topology rather than a setting, and it belongs on the same
 * page RabbitMQ's exchanges and bindings do.
 *
 * The mapping onto that page is close enough to be worth stating exactly:
 *
 *   - an exchange is a topic. It is the object a message is sent to and never
 *     kept in, and its whole job is deciding where copies go.
 *   - a binding is a rule. Its source is the topic, its destination is the
 *     subscription, and its routing key is the filter - because that is what
 *     the column means: the thing a message is matched against to decide
 *     whether it takes this edge.
 *   - the binding's properties key is the rule's name, which is genuinely the
 *     handle: rules are deleted by name, and one subscription may have several.
 *
 * Two things do not map and are refused rather than approximated. A topic has
 * no exchange type - there is no direct, fanout, topic or headers here, only
 * the rules - and a binding to another exchange has no counterpart: a rule's
 * destination is always a subscription on the topic it belongs to.
 */

// Binding argument keys this driver fills in.
//
// A contract between this package and
// frontend/src/mq/azureservicebus/rules.ts, not part of the shared vocabulary.
const (
	// ArgFilterType is "sql", "correlation" or "true". The last is what a
	// subscription's $Default rule is, and it matches everything.
	ArgFilterType = "filterType"
	// ArgExpression is the SQL filter's text. Empty on the other two kinds.
	ArgExpression = "expression"
	// ArgAction is a SQL statement run on a matching message before it is
	// copied in - the half of a rule that changes the message rather than
	// selecting it.
	ArgAction = "action"
	// ArgCorrelationPrefix carries one correlation-filter field each:
	// subject, correlationId, messageId, to, replyTo, sessionId,
	// replyToSessionId, contentType, and the sender's own properties.
	ArgCorrelationPrefix = "correlation."
)

// Filter kinds, as the routing board spells them.
const (
	FilterSQL         = "sql"
	FilterCorrelation = "correlation"
	FilterTrue        = "true"
	FilterFalse       = "false"
)

// DefaultRuleName is the rule the service adds to every new subscription. It
// matches everything, and deleting it without adding another leaves a
// subscription nothing can reach.
const DefaultRuleName = "$Default"

/*
 * ListExchanges is the namespace's topics.
 *
 * Only topics, and that is the mapping rather than a filter: a queue takes a
 * message and keeps it, which is what an exchange never does. A topic keeps
 * nothing and exists to decide where copies go.
 */
func (c *Conn) ListExchanges(ctx context.Context, _ string) ([]*model.Destination, error) {
	// The namespace argument is ignored: Service Bus has no vhost or tenant
	// inside a namespace, and one connection is one namespace.
	if err := c.live(); err != nil {
		return nil, err
	}

	topics, err := c.listTopics(ctx)
	if err != nil {
		return nil, err
	}

	names := make([][]string, len(topics))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, topic := range topics {
		group.Go(func() error {
			attached, err := c.subscriptionNames(groupCtx, topic.TopicName)
			if err != nil {
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", topic.TopicName, err)
			}
			names[index] = attached
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	exchanges := make([]*model.Destination, 0, len(topics))
	for index, topic := range topics {
		exchanges = append(exchanges,
			c.topicDestination(ctx, topic.TopicName, topic.TopicProperties, names[index]))
	}
	return exchanges, nil
}

/*
 * ListBindings is every rule in the namespace.
 *
 * Three levels of listing to assemble it - topics, then each topic's
 * subscriptions, then each subscription's rules - because the management API
 * has no call that answers any of them wholesale. The fan-out at each level is
 * bounded for the reason the destinations board's is: a request per row fired
 * together at an API that throttles arrives as a failed page rather than a
 * slow one.
 */
func (c *Conn) ListBindings(ctx context.Context, _ string) ([]*model.Binding, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	topics, err := c.listTopics(ctx)
	if err != nil {
		return nil, err
	}

	perTopic := make([][]admin.SubscriptionPropertiesItem, len(topics))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(subscriptionFanOut)
	for index, topic := range topics {
		group.Go(func() error {
			listed, err := c.listSubscriptions(groupCtx, topic.TopicName)
			if err != nil {
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s: %w", topic.TopicName, err)
			}
			perTopic[index] = listed
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	var subscriptions []admin.SubscriptionPropertiesItem
	for _, listed := range perTopic {
		subscriptions = append(subscriptions, listed...)
	}

	perSubscription := make([][]*model.Binding, len(subscriptions))
	group, groupCtx = errgroup.WithContext(ctx)
	group.SetLimit(ruleFanOut)
	for index, item := range subscriptions {
		group.Go(func() error {
			rules, err := c.listRules(groupCtx, item.TopicName, item.SubscriptionName)
			if err != nil {
				if notFound(err) {
					return nil
				}
				return fmt.Errorf("%s/%s: %w", item.TopicName, item.SubscriptionName, err)
			}
			perSubscription[index] = rules
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	bindings := make([]*model.Binding, 0, len(subscriptions))
	for _, rules := range perSubscription {
		bindings = append(bindings, rules...)
	}
	sort.Slice(bindings, func(a, b int) bool {
		if bindings[a].Source != bindings[b].Source {
			return bindings[a].Source < bindings[b].Source
		}
		if bindings[a].Destination != bindings[b].Destination {
			return bindings[a].Destination < bindings[b].Destination
		}
		return bindings[a].PropertiesKey < bindings[b].PropertiesKey
	})
	return bindings, nil
}

// listRules reads one subscription's rules as bindings.
func (c *Conn) listRules(ctx context.Context, topic, subscription string) ([]*model.Binding, error) {
	pager := c.management.NewListRulesPager(topic, subscription, nil)
	bindings := make([]*model.Binding, 0, 2)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, rule := range page.Rules {
			bindings = append(bindings, bindingOf(topic, subscription, rule))
		}
	}
	return bindings, nil
}

/*
 * bindingOf turns one rule into a canonical binding.
 *
 * RoutingKey carries the filter rather than a key, and that is what the column
 * means rather than a stretch: it is what a message is matched against to
 * decide whether it takes this edge. A SQL rule's is its expression; a
 * correlation rule's is the set of fields it compares, rendered the way the
 * service would read them.
 *
 * PropertiesKey carries the rule's name, and it is load-bearing for the same
 * reason RabbitMQ's is: a rule is deleted by name, one subscription may have
 * several, and nothing else on the row identifies which is which.
 */
func bindingOf(topic, subscription string, rule admin.RuleProperties) *model.Binding {
	arguments := map[string]string{ArgFilterType: FilterTrue}
	routingKey := ""

	switch filter := rule.Filter.(type) {
	case *admin.SQLFilter:
		arguments[ArgFilterType] = FilterSQL
		arguments[ArgExpression] = filter.Expression
		routingKey = filter.Expression
	case *admin.CorrelationFilter:
		arguments[ArgFilterType] = FilterCorrelation
		fields := correlationFields(filter)
		for name, value := range fields {
			arguments[ArgCorrelationPrefix+name] = value
		}
		routingKey = renderCorrelation(fields)
	case *admin.FalseFilter:
		arguments[ArgFilterType] = FilterFalse
		// A rule that matches nothing. It is legal and it is worth showing:
		// a subscription whose only rule is this one receives nothing, which
		// looks identical to a healthy one everywhere else.
		routingKey = "1=0"
	}

	if action, ok := rule.Action.(*admin.SQLAction); ok && action != nil {
		arguments[ArgAction] = action.Expression
	}

	return &model.Binding{
		Namespace:   "",
		Source:      topic,
		Destination: subscription,
		// Always a subscription. A rule cannot bind a topic to another topic:
		// its destination is the subscription it belongs to.
		DestinationKind: "subscription",
		RoutingKey:      routingKey,
		Arguments:       arguments,
		// The rule's name, which is what a delete takes.
		PropertiesKey: rule.Name,
	}
}

// correlationFields is the set of message fields a correlation filter compares.
//
// Only the ones it actually sets: an unset field matches anything, so listing
// it as empty would read as "matches the empty string", which is a different
// and much narrower rule.
func correlationFields(filter *admin.CorrelationFilter) map[string]string {
	fields := map[string]string{}
	put := func(name string, value *string) {
		if value != nil && *value != "" {
			fields[name] = *value
		}
	}
	put("subject", filter.Subject)
	put("correlationId", filter.CorrelationID)
	put("messageId", filter.MessageID)
	put("to", filter.To)
	put("replyTo", filter.ReplyTo)
	put("sessionId", filter.SessionID)
	put("replyToSessionId", filter.ReplyToSessionID)
	put("contentType", filter.ContentType)
	for name, value := range filter.ApplicationProperties {
		fields[name] = fmt.Sprint(value)
	}
	return fields
}

// renderCorrelation writes a correlation filter the way its SQL equivalent
// would read, so the routing column says the same thing for both kinds.
func renderCorrelation(fields map[string]string) string {
	if len(fields) == 0 {
		// A correlation filter setting nothing matches everything, which is
		// what $Default is - and saying so is better than an empty cell.
		return "1=1"
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s = '%s'", name, fields[name]))
	}
	return strings.Join(parts, " AND ")
}

/*
 * DeclareExchange creates a topic, which is what an exchange is here.
 *
 * The type is refused rather than ignored. A RabbitMQ exchange's type is the
 * whole of how it routes - direct, fanout, topic, headers - and a Service Bus
 * topic has none: what routes is the rules on its subscriptions. Accepting one
 * silently would let a form claim a fanout topic had been created.
 */
func (c *Conn) DeclareExchange(ctx context.Context, spec model.ExchangeSpec) error {
	if kind := strings.TrimSpace(spec.Type); kind != "" {
		return fmt.Errorf(
			"a service bus topic has no exchange type; %q means nothing here, and what "+
				"decides where a message goes is the rules on its subscriptions", kind)
	}
	if spec.Transient || spec.AutoDelete {
		// Neither is a topic's own setting. Auto-delete on idle exists and is
		// a duration rather than a flag, and it is on the entity form.
		return errors.New(
			"a service bus topic is durable and is not auto-deleted by a flag; " +
				"auto-delete on idle is a duration, set on the queues and topics page")
	}
	return c.CreateDestination(ctx, model.DestinationSpec{
		Ref:        model.DestinationRef{Name: spec.Name},
		Attributes: map[string]string{AttrEntityType: EntityTopic},
	})
}

// RemoveExchange deletes a topic, and with it every subscription on it and
// whatever those had not delivered.
func (c *Conn) RemoveExchange(ctx context.Context, _, name string) error {
	return c.RemoveDestination(ctx, model.DestinationRef{Name: name})
}

/*
 * DeclareBinding creates a rule.
 *
 * Create rather than create-or-update: the service has a separate update call
 * and this one refuses a name that exists, which is what a form adding a rule
 * beside an existing one wants. Editing one is a delete and a create, because
 * a rule's filter kind cannot change - a SQL rule and a correlation rule are
 * different objects wearing one name.
 */
func (c *Conn) DeclareBinding(ctx context.Context, binding model.Binding) error {
	if err := c.live(); err != nil {
		return err
	}
	topic, err := requiredName("topic", binding.Source)
	if err != nil {
		return err
	}
	subscription, err := requiredName("subscription", binding.Destination)
	if err != nil {
		return errors.New("a rule belongs to one subscription, and this one names none")
	}
	name, err := requiredRuleName(binding.PropertiesKey)
	if err != nil {
		return errors.New(
			"a rule has a name, and it is what deletes it: one subscription may have several")
	}

	options, err := ruleOptions(name, binding)
	if err != nil {
		return err
	}

	_, err = c.management.CreateRule(ctx, topic, subscription, options)
	switch {
	case alreadyExists(err):
		return fmt.Errorf("a rule named %q already exists on %s/%s", name, topic, subscription)
	case notFound(err):
		return fmt.Errorf("no subscription named %q on %s in %s",
			subscription, topic, c.config.namespace)
	}
	return err
}

/*
 * RemoveBinding deletes a rule.
 *
 * The one worth thinking twice about is the last one: a subscription with no
 * rules receives nothing at all. The service allows it, reports the
 * subscription as Active, and shows an empty backlog because nothing can
 * arrive - so this driver's subscriptions board reports that state as offline
 * rather than letting it look healthy.
 */
func (c *Conn) RemoveBinding(ctx context.Context, binding model.Binding) error {
	if err := c.live(); err != nil {
		return err
	}
	topic, err := requiredName("topic", binding.Source)
	if err != nil {
		return err
	}
	subscription, err := requiredName("subscription", binding.Destination)
	if err != nil {
		return errors.New("a rule belongs to one subscription, and this one names none")
	}
	name, err := requiredRuleName(binding.PropertiesKey)
	if err != nil {
		return errors.New("a rule is deleted by name, and this one names none")
	}

	_, err = c.management.DeleteRule(ctx, topic, subscription, name, nil)
	if notFound(err) {
		return fmt.Errorf("no rule named %q on %s/%s", name, topic, subscription)
	}
	return err
}

/*
 * requiredRuleName trims a rule name, allowing the one the service owns.
 *
 * requiredName refuses a leading $ because on an entity it would address a
 * sub-entity such as $DeadLetterQueue. A rule is a different name space and
 * $Default is a real rule in it - the one every subscription is created with -
 * so deleting or replacing it has to be possible.
 */
func requiredRuleName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == DefaultRuleName {
		return trimmed, nil
	}
	return requiredName("rule", trimmed)
}

// ruleOptions turns a binding into the rule the API takes.
//
// The filter kind comes from the arguments rather than being guessed from the
// routing key, because the two kinds accept different text and a guess would
// send a correlation filter's fields as a SQL expression.
func ruleOptions(name string, binding model.Binding) (*admin.CreateRuleOptions, error) {
	options := &admin.CreateRuleOptions{Name: &name}

	switch strings.TrimSpace(binding.Arguments[ArgFilterType]) {
	case FilterSQL:
		expression := strings.TrimSpace(binding.Arguments[ArgExpression])
		if expression == "" {
			expression = strings.TrimSpace(binding.RoutingKey)
		}
		if expression == "" {
			return nil, errors.New(
				"a SQL rule needs an expression, such as colour = 'red'")
		}
		options.Filter = &admin.SQLFilter{Expression: expression}
	case FilterCorrelation:
		filter, given := correlationFilterOf(binding.Arguments)
		if !given {
			return nil, errors.New(
				"a correlation rule needs at least one field to match, such as the subject")
		}
		options.Filter = filter
	case FilterFalse:
		options.Filter = &admin.FalseFilter{}
	default:
		// A rule with no filter kind is one that matches everything, which is
		// what the service's own $Default is.
		options.Filter = &admin.TrueFilter{}
	}

	if action := strings.TrimSpace(binding.Arguments[ArgAction]); action != "" {
		options.Action = &admin.SQLAction{Expression: action}
	}
	return options, nil
}

// correlationFilterOf reads the fields a correlation rule compares.
//
// Only the system fields it knows go on the filter itself; anything else is a
// sender-set property, which the filter matches through a table of its own.
func correlationFilterOf(arguments map[string]string) (*admin.CorrelationFilter, bool) {
	filter := &admin.CorrelationFilter{}
	given := false
	set := func(target **string, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			*target = &trimmed
			given = true
		}
	}

	for key, value := range arguments {
		if !strings.HasPrefix(key, ArgCorrelationPrefix) {
			continue
		}
		field := strings.TrimPrefix(key, ArgCorrelationPrefix)
		switch field {
		case "subject":
			set(&filter.Subject, value)
		case "correlationId":
			set(&filter.CorrelationID, value)
		case "messageId":
			set(&filter.MessageID, value)
		case "to":
			set(&filter.To, value)
		case "replyTo":
			set(&filter.ReplyTo, value)
		case "sessionId":
			set(&filter.SessionID, value)
		case "replyToSessionId":
			set(&filter.ReplyToSessionID, value)
		case "contentType":
			set(&filter.ContentType, value)
		default:
			if strings.TrimSpace(value) == "" {
				continue
			}
			if filter.ApplicationProperties == nil {
				filter.ApplicationProperties = map[string]any{}
			}
			filter.ApplicationProperties[field] = value
			given = true
		}
	}
	return filter, given
}

/*
 * RuleSpec is a rule as the routing form collects it.
 *
 * Deliberately not ExchangeSpec or Binding. A binding is RabbitMQ's shape - a
 * source, a destination, a routing key and a table of arguments - and a rule
 * is a filter of one of three kinds plus an optional action, which is a
 * different thing to fill in even though it lands on the same page.
 */
type RuleSpec struct {
	Topic        string
	Subscription string
	// Name is what deletes it: one subscription may have several rules, and
	// nothing else tells them apart.
	Name string

	// Kind is "sql", "correlation", "true" or "false". Empty means true,
	// which is what the service's own $Default rule is.
	Kind string

	// Expression is the SQL filter's text, on a sql rule.
	Expression string
	// Correlation is the set of message fields a correlation rule compares.
	// A field left out matches anything; a field set to the empty string
	// would match only the empty string, so empties are dropped.
	Correlation map[string]string

	// Action is a SQL statement run on a matching message before it is copied
	// in. It is the half of a rule that changes the message rather than
	// selecting it, and it is optional on every kind.
	Action string
}

func (s RuleSpec) binding() model.Binding {
	arguments := map[string]string{ArgFilterType: strings.TrimSpace(s.Kind)}
	if expression := strings.TrimSpace(s.Expression); expression != "" {
		arguments[ArgExpression] = expression
	}
	if action := strings.TrimSpace(s.Action); action != "" {
		arguments[ArgAction] = action
	}
	for field, value := range s.Correlation {
		if strings.TrimSpace(field) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		arguments[ArgCorrelationPrefix+field] = value
	}
	return model.Binding{
		Source:          strings.TrimSpace(s.Topic),
		Destination:     strings.TrimSpace(s.Subscription),
		DestinationKind: "subscription",
		PropertiesKey:   strings.TrimSpace(s.Name),
		Arguments:       arguments,
	}
}

// CreateRule declares a rule from a form submission.
func (c *Conn) CreateRule(ctx context.Context, spec RuleSpec) error {
	return c.DeclareBinding(ctx, spec.binding())
}

// RemoveRule deletes one rule by name.
func (c *Conn) RemoveRule(ctx context.Context, topic, subscription, name string) error {
	return c.RemoveBinding(ctx, model.Binding{
		Source:        topic,
		Destination:   subscription,
		PropertiesKey: name,
	})
}
