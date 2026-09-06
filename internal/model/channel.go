package model

// The shape only IBM MQ has.
//
// It lives here beside the other families' own files - policy.go and
// replication.go are RabbitMQ's, redisstream.go is Redis Streams', shard.go is
// Kinesis's - because the canonical vocabulary is what several families share,
// not the only thing that may be named.
//
// A channel exists as a type of its own because nothing already here could
// carry one, and the three near misses are worth naming.
//
// It is not a ClientConnection. That is the transport a connected application
// is holding right now: it appears when the application connects, disappears
// when it goes, and is addressed by a peer address. A channel is a definition
// an administrator made. It exists with nothing connected, it is what decides
// whether a connection is allowed at all, and one channel definition can have
// many running instances at once - so a page built for connections would show
// nothing on a queue manager whose applications are all idle, which is exactly
// when somebody is looking for why they cannot connect.
//
// It is not a Destination either. Nothing is stored in a channel and nothing
// is addressed through one; a sender channel names a transmission queue, which
// is a destination, and the channel is the thing that drains it.
//
// And it is not a Subscription: it carries no position, no backlog and no
// consumer group.
//
// What it is instead is the answer to "how does anything reach this queue
// manager": a name, a type that says which direction it works in, an address,
// a status that is not derivable from anything else on any other page, and the
// counters that say whether it has ever carried a message.

// ChannelType is which direction a channel works in, and what is at the other
// end. The values are the family's own words, lower-camelled by the API.
type ChannelType string

const (
	// ChannelServerConnection is how a client application connects. It is the
	// one type with no partner queue manager: an application connects to it,
	// and there may be hundreds of instances of one definition at once.
	ChannelServerConnection ChannelType = "serverConnection"
	// ChannelClientConnection is the other half of that pair, and it is a
	// definition this queue manager holds on behalf of clients rather than one
	// it runs. It never has a status.
	ChannelClientConnection ChannelType = "clientConnection"

	// The message channels, which move messages between queue managers. A
	// sender drains a transmission queue towards a receiver; a server and a
	// requester are the same pair started from the other end.
	ChannelSender    ChannelType = "sender"
	ChannelReceiver  ChannelType = "receiver"
	ChannelServer    ChannelType = "server"
	ChannelRequester ChannelType = "requester"

	// The cluster pair, which a queue manager starts for itself: a cluster
	// sender is often defined automatically from a repository's information
	// rather than by an administrator.
	ChannelClusterSender   ChannelType = "clusterSender"
	ChannelClusterReceiver ChannelType = "clusterReceiver"

	// ChannelAMQP is the queue manager's AMQP 1.0 listener, which is a channel
	// here for the same reason a server-connection channel is: it is where a
	// client arrives.
	ChannelAMQP ChannelType = "amqp"
)

// ChannelStatus is what a channel is doing now.
//
// Empty means the queue manager reported no status at all, which is not the
// same as stopped: a channel that has never been started has no status object,
// and a client-connection channel never has one. The distinction is the whole
// value of the page - "inactive" is normal for a channel nobody uses, and
// "retrying" on the same channel means something is wrong.
type ChannelStatus string

const (
	ChannelInactive     ChannelStatus = "inactive"
	ChannelRunning      ChannelStatus = "running"
	ChannelStarting     ChannelStatus = "starting"
	ChannelBinding      ChannelStatus = "binding"
	ChannelInitializing ChannelStatus = "initializing"
	ChannelRetrying     ChannelStatus = "retrying"
	ChannelStopping     ChannelStatus = "stopping"
	ChannelStopped      ChannelStatus = "stopped"
	ChannelPaused       ChannelStatus = "paused"
	ChannelRequesting   ChannelStatus = "requesting"
	ChannelSwitching    ChannelStatus = "switching"
)

// Channel is one channel definition and the aggregate state of its instances.
type Channel struct {
	Name string      `json:"name"`
	Type ChannelType `json:"type"`

	// Description is what the definition says about itself.
	Description string `json:"description"`

	// ConnectionName is where this channel dials, or where its running
	// instance connected from - which of the two depends on the type, and the
	// difference is worth keeping rather than splitting into two fields
	// nobody could fill for the other half. A sender's is configured; a
	// receiver's exists only while something is connected.
	ConnectionName string `json:"connectionName"`

	// TransmissionQueue is the queue a sender or server channel drains.
	// Empty on every other type, and empty on a sender is a misconfiguration
	// rather than a default.
	TransmissionQueue string `json:"transmissionQueue"`

	// Status is the definition's overall state. Where several instances are
	// running it is the one furthest from healthy, because a page showing
	// "running" for a definition with one instance retrying would hide the
	// only row worth looking at.
	Status ChannelStatus `json:"status"`

	// Substate is what a running instance is waiting on - receiving, sending,
	// committing, resolving a name. It is the field that separates a channel
	// that is busy from one that is stuck, and both report "running".
	Substate string `json:"substate"`

	// Instances is how many are running. A server-connection channel is one
	// definition and one instance per connected application, so this is the
	// only place a client count appears for this family.
	Instances int `json:"instances"`

	// RemoteQueueManager is what answered at the other end. Empty until a
	// channel has connected at least once, which is why a sender that has
	// never worked shows a connection name and no partner.
	RemoteQueueManager string `json:"remoteQueueManager"`

	// Messages, BytesSent and BytesReceived are this instance's totals since
	// it started, not the definition's lifetime. They reset when a channel
	// restarts, which is worth knowing before reading a zero as "never used".
	// UnknownMetric where nothing is running.
	Messages      int64 `json:"messages"`
	BytesSent     int64 `json:"bytesSent"`
	BytesReceived int64 `json:"bytesReceived"`

	// StartedAt is when the running instance started, as the queue manager
	// spells it. It is text rather than an instant because MQ prints its own
	// local date and clock with no zone at all.
	StartedAt string `json:"startedAt"`
	// LastMessageAt is when a message last crossed. Empty on a channel that
	// has carried none, which on a sender that is running is the interesting
	// case.
	LastMessageAt string `json:"lastMessageAt"`

	// InDoubt is a message channel that sent a batch and never learned whether
	// the other end committed it. Nothing else on any page reports it, and it
	// is the state that needs a human: the batch is neither delivered nor
	// resendable until somebody resolves it.
	InDoubt bool `json:"inDoubt"`

	// StopRequested is an operator having asked a running channel to stop,
	// which it will do at the end of its current batch. Without it a channel
	// that is quietly winding down looks identical to one that is working.
	StopRequested bool `json:"stopRequested"`
}
