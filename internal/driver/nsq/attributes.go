package nsq

// The attribute keys this driver puts on a Destination or a Subscription.
//
// A contract between this package and frontend/src/mq/nsq, not part of the
// shared vocabulary. One set for both, because nsqd reports a topic and a
// channel with largely the same figures and the two boards read them the same
// way; the keys only one of them carries are grouped below.
const (
	AttrPaused       = "paused"
	AttrBackendDepth = "backendDepth"
	AttrMessageCount = "messageCount"
	AttrInFlight     = "inFlight"
	AttrDeferred     = "deferred"
	AttrRequeued     = "requeued"
	AttrTimedOut     = "timedOut"
	AttrEphemeral    = "ephemeral"
	AttrNodes        = "nodes"
)

// A topic's alone. The two depth keys are the split behind the canonical
// Depth: a board that showed only the total would leave a reader unable to
// tell a topic nothing has consumed from one that is paused.
const (
	AttrTopicDepth   = "topicDepth"
	AttrChannelDepth = "channelDepth"
	AttrMessageBytes = "messageBytes"
	AttrChannels     = "channels"
)

// A channel's alone. The topic is part of a channel's identity rather than a
// label on it: two topics with a channel of the same name have nothing in
// common.
const AttrTopic = "topic"

// ephemeralSuffix marks a topic or channel that exists only while something is
// connected to it. nsqd deletes it when the last client goes.
const ephemeralSuffix = "#ephemeral"
