package nsq

// The shapes nsqd and nsqlookupd answer with, named for what they are rather
// than for the endpoint that returns them.
//
// Every field here was read off a running 1.3.0 rather than off the docs. The
// two that are easy to get wrong are named in comments: depth is per daemon
// and never a cluster total, and a count is cumulative since the daemon
// started rather than a rate.

// nsqdInfo is nsqd's /info. It is also how this driver tells an nsqd from an
// nsqlookupd: only nsqd reports a tcp_port here.
type nsqdInfo struct {
	Version          string `json:"version"`
	BroadcastAddress string `json:"broadcast_address"`
	Hostname         string `json:"hostname"`
	HTTPPort         int    `json:"http_port"`
	TCPPort          int    `json:"tcp_port"`
	StartTime        int64  `json:"start_time"`

	// Durations, in nanoseconds, as Go marshals a time.Duration.
	MaxHeartbeatInterval   int64 `json:"max_heartbeat_interval"`
	MaxOutputBufferTimeout int64 `json:"max_output_buffer_timeout"`
	MaxOutputBufferSize    int64 `json:"max_output_buffer_size"`
	MaxDeflateLevel        int   `json:"max_deflate_level"`
}

// nsqdStats is nsqd's /stats?format=json, for this daemon only.
type nsqdStats struct {
	Version   string        `json:"version"`
	Health    string        `json:"health"`
	StartTime int64         `json:"start_time"`
	Topics    []topicStats  `json:"topics"`
	Memory    memoryStats   `json:"memory"`
	Producers []clientStats `json:"producers"`
}

// topicStats is one topic on one nsqd.
type topicStats struct {
	Name string `json:"topic_name"`
	// Depth is what this topic holds that no channel has taken yet, which on
	// a topic with any channel at all is normally zero: nsqd copies a message
	// into every channel as it arrives.
	Depth        int64          `json:"depth"`
	BackendDepth int64          `json:"backend_depth"`
	MessageCount uint64         `json:"message_count"`
	MessageBytes uint64         `json:"message_bytes"`
	Paused       bool           `json:"paused"`
	Channels     []channelStats `json:"channels"`
}

// channelStats is one channel of one topic on one nsqd. The depth here is the
// backlog: what has been published and not yet finished by a consumer of this
// channel.
type channelStats struct {
	Name          string        `json:"channel_name"`
	Depth         int64         `json:"depth"`
	BackendDepth  int64         `json:"backend_depth"`
	InFlightCount int           `json:"in_flight_count"`
	DeferredCount int           `json:"deferred_count"`
	MessageCount  uint64        `json:"message_count"`
	RequeueCount  uint64        `json:"requeue_count"`
	TimeoutCount  uint64        `json:"timeout_count"`
	ClientCount   int           `json:"client_count"`
	Paused        bool          `json:"paused"`
	Clients       []clientStats `json:"clients"`
}

// clientStats is one connected consumer of one channel.
type clientStats struct {
	ClientID      string `json:"client_id"`
	Hostname      string `json:"hostname"`
	Version       string `json:"version"`
	RemoteAddress string `json:"remote_address"`
	UserAgent     string `json:"user_agent"`

	// State is nsqd's connection state machine, as a number: 3 is subscribed,
	// which is the only one a channel's client list can hold.
	State int `json:"state"`
	// ReadyCount is what this consumer told nsqd it will accept. A zero here
	// on a channel with a backlog is the whole explanation for a consumer
	// that is connected and taking nothing.
	ReadyCount    int    `json:"ready_count"`
	InFlightCount int    `json:"in_flight_count"`
	MessageCount  uint64 `json:"message_count"`
	FinishCount   uint64 `json:"finish_count"`
	RequeueCount  uint64 `json:"requeue_count"`
	ConnectTS     int64  `json:"connect_ts"`

	TLS    bool `json:"tls"`
	Snappy bool `json:"snappy"`

	TLSCipherSuite string `json:"tls_cipher_suite"`
	TLSVersion     string `json:"tls_version"`
}

// memoryStats is the Go runtime's own figures, which nsqd reports because it
// has no other storage to account for: a topic is a memory queue with a disk
// overflow behind it.
type memoryStats struct {
	HeapObjects     uint64 `json:"heap_objects"`
	HeapIdleBytes   uint64 `json:"heap_idle_bytes"`
	HeapInUseBytes  uint64 `json:"heap_in_use_bytes"`
	HeapReleased    uint64 `json:"heap_released_bytes"`
	NextGCBytes     uint64 `json:"next_gc_bytes"`
	GCTotalRuns     uint64 `json:"gc_total_runs"`
	GCPauseUsec99   uint64 `json:"gc_pause_usec_99"`
	GCPauseUsec100  uint64 `json:"gc_pause_usec_100"`
	GCPauseUsec95th uint64 `json:"gc_pause_usec_95"`
}

// lookupProducer is one nsqd as nsqlookupd knows it.
//
// The address here is the one the nsqd broadcast about itself, which is not
// necessarily one this machine can reach - so it names a node on the
// directory board and is never dialled.
type lookupProducer struct {
	RemoteAddress    string   `json:"remote_address"`
	Hostname         string   `json:"hostname"`
	BroadcastAddress string   `json:"broadcast_address"`
	TCPPort          int      `json:"tcp_port"`
	HTTPPort         int      `json:"http_port"`
	Version          string   `json:"version"`
	Tombstones       []bool   `json:"tombstones"`
	Topics           []string `json:"topics"`
}

// lookupNodes is nsqlookupd's /nodes.
type lookupNodes struct {
	Producers []lookupProducer `json:"producers"`
}

// lookupTopics is nsqlookupd's /topics.
type lookupTopics struct {
	Topics []string `json:"topics"`
}

// lookupChannels is nsqlookupd's /channels?topic=.
type lookupChannels struct {
	Channels []string `json:"channels"`
}

// lookupInfo is nsqlookupd's /info, which carries a version and nothing else.
// That emptiness is load-bearing: it is what tells the two daemons apart.
type lookupInfo struct {
	Version string `json:"version"`
}
