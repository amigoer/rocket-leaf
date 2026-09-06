package model

// The shape only Amazon Kinesis has.
//
// It lives here beside the other families' own files - policy.go and
// replication.go are RabbitMQ's, redisstream.go is Redis Streams' - because
// the canonical vocabulary is what several families share, not the only thing
// that may be named.
//
// A shard exists as a type of its own because Destination.Partitions could not
// carry one. That field is an int, and a count is a true thing to say about a
// stream - it really is divided into N shards taking writes - but it is the
// only true thing a number can say. Every other family's partitions are
// interchangeable slots addressed by index; a shard is named, owns a slice of
// the hash space that decides which records land on it, has a read quota of
// its own, and is changed by being split in two or merged with a neighbour
// rather than by being resized. The shard that was split is not gone: it stops
// taking writes, keeps the records already on it until retention expires, and
// its children name it as their parent.
//
// Squeezing that into a partition index would lose the three things anyone
// opens the page for - which shard a partition key lands on, why a stream has
// more shards than its count says, and where the records of a shard that no
// longer takes writes went.

// Shard is one part of a stream, open or closed.
type Shard struct {
	ID string `json:"id"`

	// ParentID is the shard this one was split from or merged out of, and
	// AdjacentParentID is the second parent a merge has. Both empty means the
	// shard was there when the stream was created.
	//
	// They are what makes a listing a lineage rather than a set: a consumer
	// that has to read a stream in order has to finish a parent before it
	// starts a child, and the parent is the only place that says so.
	ParentID         string `json:"parentId"`
	AdjacentParentID string `json:"adjacentParentId"`

	// StartHashKey and EndHashKey bound the slice of the key space this shard
	// takes. They are 128-bit unsigned integers, so they are carried as
	// decimal strings: the largest of them does not fit in an int64, and a
	// float would round the low digits away - which are exactly the digits
	// that decide which of two neighbouring shards a key lands on.
	StartHashKey string `json:"startHashKey"`
	EndHashKey   string `json:"endHashKey"`

	// StartSequence is the first sequence number in the shard, and
	// EndSequence is the last. EndSequence is set only on a closed shard, and
	// setting it is what closing means.
	StartSequence string `json:"startSequence"`
	EndSequence   string `json:"endSequence"`

	// Closed is true for a shard that takes no more writes because it was
	// split or merged. It still holds its records until retention expires, so
	// it is not the same as deleted and must not be hidden - a stream that
	// looks like it lost data usually just has a closed parent nobody drained.
	Closed bool `json:"closed"`
}
