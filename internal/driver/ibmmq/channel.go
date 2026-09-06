package ibmmq

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/amigoer/mq-studio/internal/model"
)

/*
 * Channels, and why they come from MQSC rather than from the REST resource.
 *
 * There is a /admin/qmgr/{qmgr}/channel resource, and it is not enough. It
 * answers for message channels only - sender, receiver, server, requester and
 * the cluster pair - and returns nothing at all for a server-connection
 * channel, which is how every client application reaches the queue manager and
 * therefore the row anybody opens this page to look at. It also has no status
 * sub-resource: the query parameter exists and comes back as an empty array on
 * every channel, running or not.
 *
 * DISPLAY CHANNEL and DISPLAY CHSTATUS have neither gap, so this is two MQSC
 * calls: the definitions, and the status of whatever is running. They are
 * joined by name here rather than asked for per channel, because a status
 * display for one channel is one round trip and a queue manager with thirty
 * definitions would otherwise cost thirty.
 */

// The MQSC attributes each half asks for.
//
// Named rather than "all", for the reason the queue listing names its own: a
// bare DISPLAY CHANNEL(*) ALL returns some seventy attributes per channel
// including every exit name and cipher setting, and this page reads six.
var channelAttributes = []string{"chltype", "conname", "descr", "xmitq", "altdate", "alttime"}

// The status half asks for all of them, which is the exception here.
//
// runCommandJSON validates every named attribute against a list of its own per
// object type, and its list for channel status does not include msgs,
// bytssent or bytsrcvd - it refuses the request outright rather than leaving
// them out. Those three come back when the whole record is asked for, so this
// asks for the whole record. It costs nothing a listing would notice: there is
// one of these per running instance, not one per definition.
var channelStatusAll = []string{"all"}

/*
 * ListChannels enumerates every channel and folds in what is running.
 *
 * A definition can have several instances at once - a server-connection
 * channel has one per connected application - so the status rows are grouped
 * by name rather than matched one to one. What a definition then reports is
 * the count, and the status furthest from healthy: a page showing "running"
 * for a definition whose third instance is retrying would hide the only row
 * worth looking at.
 */
func (c *Conn) ListChannels(ctx context.Context) ([]*model.Channel, error) {
	if err := c.live(); err != nil {
		return nil, err
	}

	definitions, err := c.display(ctx, "channel", "*", channelAttributes...)
	if err != nil {
		return nil, err
	}
	// A queue manager with nothing running answers this with "not found",
	// which display() already turns into an empty result rather than an error.
	running, err := c.display(ctx, "chstatus", "*", channelStatusAll...)
	if err != nil {
		return nil, err
	}

	instances := make(map[string][]map[string]json.RawMessage, len(running))
	for _, status := range running {
		name := mqscString(status, "channel")
		instances[name] = append(instances[name], status)
	}

	channels := make([]*model.Channel, 0, len(definitions))
	for _, definition := range definitions {
		name := mqscString(definition, "channel")
		if name == "" {
			continue
		}
		channels = append(channels, channelOf(name, definition, instances[name]))
	}

	sort.Slice(channels, func(i, j int) bool { return channels[i].Name < channels[j].Name })
	return channels, nil
}

// channelOf folds one definition and its running instances into one row.
func channelOf(name string, definition map[string]json.RawMessage, instances []map[string]json.RawMessage) *model.Channel {
	channel := &model.Channel{
		Name:              name,
		Type:              channelType(mqscString(definition, "chltype")),
		Description:       mqscString(definition, "descr"),
		ConnectionName:    mqscString(definition, "conname"),
		TransmissionQueue: mqscString(definition, "xmitq"),
		Instances:         len(instances),
		// UnknownMetric rather than zero where nothing is running: a channel
		// that has never started has carried no messages and a channel running
		// with a count of zero has carried none either, and only one of those
		// is a problem.
		Messages:      model.UnknownMetric,
		BytesSent:     model.UnknownMetric,
		BytesReceived: model.UnknownMetric,
	}

	if len(instances) == 0 {
		// No status object at all. Not the same as stopped: a channel nobody
		// has started has none, and a client-connection channel never has one
		// because this queue manager does not run it.
		return channel
	}

	worst := instances[0]
	for _, instance := range instances[1:] {
		if statusRank(mqscString(instance, "status")) > statusRank(mqscString(worst, "status")) {
			worst = instance
		}
	}

	channel.Status = channelStatusOf(mqscString(worst, "status"))
	channel.Substate = strings.ToLower(mqscString(worst, "substate"))
	channel.RemoteQueueManager = mqscString(worst, "rqmname")
	channel.InDoubt = strings.EqualFold(mqscString(worst, "indoubt"), "yes")
	channel.StopRequested = strings.EqualFold(mqscString(worst, "stopreq"), "yes")
	channel.StartedAt = joinDateAndTime(mqscString(worst, "chstada"), mqscString(worst, "chstati"))
	channel.LastMessageAt = joinDateAndTime(mqscString(worst, "lstmsgda"), mqscString(worst, "lstmsgti"))

	// A running instance's connection name is where the partner is, which is
	// the useful answer for a receiver and a server-connection channel - the
	// definition has none for either.
	if connection := mqscString(worst, "conname"); connection != "" {
		channel.ConnectionName = connection
	}
	if transmission := mqscString(worst, "xmitq"); transmission != "" {
		channel.TransmissionQueue = transmission
	}

	// Summed across instances, because one definition's traffic is what a
	// reader is after and a server-connection channel's is spread over every
	// connected application.
	channel.Messages = sumAcross(instances, "msgs")
	channel.BytesSent = sumAcross(instances, "bytssent")
	channel.BytesReceived = sumAcross(instances, "bytsrcvd")
	return channel
}

func sumAcross(instances []map[string]json.RawMessage, key string) int64 {
	total := int64(0)
	found := false
	for _, instance := range instances {
		if value, ok := mqscInt(instance, key); ok {
			total += value
			found = true
		}
	}
	if !found {
		return model.UnknownMetric
	}
	return total
}

/*
 * statusRank orders the states by how much they want attention.
 *
 * It is not an ordering of the channel's lifecycle. What it decides is which
 * instance's state a definition reports when several are running, and the
 * answer has to be the worst one: a server-connection channel with forty
 * healthy instances and one stuck in doubt should not read as "running".
 */
func statusRank(status string) int {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "RUNNING":
		return 0
	case "REQUESTING", "SWITCHING":
		return 1
	case "STARTING", "INITIALIZING", "BINDING":
		return 2
	case "PAUSED", "STOPPING":
		return 3
	case "STOPPED":
		return 4
	case "RETRYING":
		return 5
	default:
		return 6
	}
}

// channelType lower-camels MQSC's own abbreviations.
//
// The mapping is explicit rather than computed because the abbreviations are
// not shortenings of one scheme: SVRCONN and CLNTCONN are the two halves of
// one pair, RQSTR drops its vowels, and CLUSSDR is two words run together.
func channelType(raw string) model.ChannelType {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SVRCONN":
		return model.ChannelServerConnection
	case "CLNTCONN":
		return model.ChannelClientConnection
	case "SDR":
		return model.ChannelSender
	case "RCVR":
		return model.ChannelReceiver
	case "SVR":
		return model.ChannelServer
	case "RQSTR":
		return model.ChannelRequester
	case "CLUSSDR":
		return model.ChannelClusterSender
	case "CLUSRCVR":
		return model.ChannelClusterReceiver
	case "AMQP":
		return model.ChannelAMQP
	default:
		// Passed through rather than dropped: a type a newer queue manager
		// grows should reach the page as its own word rather than as blank.
		return model.ChannelType(strings.ToLower(strings.TrimSpace(raw)))
	}
}

func channelStatusOf(raw string) model.ChannelStatus {
	return model.ChannelStatus(strings.ToLower(strings.TrimSpace(raw)))
}

// joinDateAndTime puts back together what MQSC splits across two attributes.
//
// The result is text rather than an instant: MQ prints the queue manager's own
// local date and clock with no zone, and turning that into a moment would be
// claiming an offset nobody stated. The clock is dotted - 07.41.32 - which
// reads as a version number, so it is re-spelled with colons.
func joinDateAndTime(date, clock string) string {
	clock = strings.ReplaceAll(clock, ".", ":")
	switch {
	case date == "":
		return clock
	case clock == "":
		return date
	default:
		return date + " " + clock
	}
}
