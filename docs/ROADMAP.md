# Roadmap

[简体中文](ROADMAP.zh-CN.md)

MQ Studio is becoming one desktop client for every message queue. Every broker family is
reached through a pluggable driver that declares its own capabilities, and the interface
only offers what the connected endpoint can actually do.

This is the delivery plan. The contract it delivers against is
[the multi-MQ design](MULTI_MQ_DESIGN.md); the short user-facing status table lives in the
[README](../README.md).

## Where things stand

- **Shipped** — RocketMQ 4.x / 5.x through the Admin API, feature-complete.
- **Shipped** — RabbitMQ 3.x / 4.x through the HTTP management plugin, with the data plane on
  AMQP 0-9-1 rather than the management API's publish and get endpoints. The whole management
  plane: queues with their full arguments, exchanges and bindings, connections and channels,
  dead letters, nodes with health checks and feature flags, virtual hosts, users and
  permissions, policies and parameters, definitions import and export, shovels and federation,
  and stream queues.
- **Shipped** — Kafka 3.x / 4.x over the Kafka protocol itself, through franz-go and its kadm
  package. Topics with their partitions, replicas and settings; consumer groups with
  per-partition lag and all five of Kafka's offset resets; browsing a log by offset, timestamp
  or key and following its end; producing with a key, headers, a pinned partition and a chosen
  acknowledgement level; brokers with their effective settings and their log directories; ACLs
  and SCRAM users; client quotas; partition reassignment with preferred-leader election; and the
  transactions a cluster is tracking, so a pipeline stopped by a producer that died
  mid-transaction is visible somewhere.
  Broker settings are read-only. Everything needed to write them is in place - the driver reads
  them through the same incremental-alter path a topic's settings use - but the page offers no
  editor, and a cluster-wide setting and a per-broker override are different writes that deserve
  to be told apart before either is offered.

  Three things it deliberately does not have. There is no dead-letter page: Kafka has no
  broker-side dead-letter queue, and the .DLT suffix is Spring Kafka's convention rather than
  Kafka's. There is no rate anywhere: the admin protocol reports none, so a produce or consume
  rate would have to be invented or read from JMX, which this app does not speak. And there is
  no disk percentage: Kafka reports the bytes its partitions occupy and nothing about the
  filesystem holding them, so there is no denominator to build one from.

- **Shipped** — Redis Stream 6.0+ over the Redis protocol itself, through go-redis. Streams
  with their length, memory and entry range; consumer groups with lag and every reposition
  XGROUP SETID offers; browsing entries by time window or by id, and writing them as the
  ordered field lists they are; the pending entries list with claim, auto-claim and
  acknowledge; the server's memory, persistence and slow log; its client connections; and ACL
  users with their key, channel and command rules. Standalone, sentinel and cluster all
  connect, and a cluster's streams are listed from every master rather than from the node that
  was dialled.

  The pending entries list stands in for the dead-letter page, because Redis moves nothing
  aside: an entry handed to a consumer stays in the stream and stays owed to that consumer
  until it is acknowledged or claimed away. No message rate and no disk figure are reported
  anywhere - Redis counts commands rather than messages, and reports memory rather than
  disk.

- **Shipped** — Apache Pulsar 3.x / 4.x, over the binary protocol for data and the admin REST
  API for everything else. Topics with their partitions and storage kind, the namespaces and
  tenants above them, subscriptions with their backlog and cursor, a message browser and live
  tail, a send console, brokers, dead-letter topics, role grants and alerts. The two planes are
  reported separately: a connection whose web service answers while its broker port does not can
  still read every page, and says why it cannot publish.

  Pulsar's own vocabulary rather than a translation of another family's. There is no tag
  anywhere - what a RocketMQ producer puts in one, a Pulsar producer puts in a property. A
  subscription is a stored cursor that exists without a consumer attached, so an idle one is a
  normal state rather than a group that has gone away. The tokens page lists role grants rather
  than accounts, because Pulsar authorises the subject of a token and keeps no directory of
  them. And it has a blocked-subscription alert no other family needs: past its unacknowledged
  limit the broker stops delivering entirely, which from the backlog alone is indistinguishable
  from a slow consumer.

- **Shipped** — MQTT 3.1.1 and 5.0, over Paho's two Go libraries, which do not overlap: one
  speaks 3.1.1 and the other 5.0, and a broker configured for either refuses the other's
  CONNECT. The first family here with no administrative plane of its own, so what a connection
  can do is decided when it dials, in three tiers — the protocol, the $SYS tree most brokers
  publish, and the REST API EMQX and its peers add. Publishing with QoS, retain and the 5.0
  properties; a live subscribe workbench that reports what it dropped and when the session went
  down; topics from the retained set, which is the only thing MQTT can enumerate; broker
  counters from $SYS; and, where a management API answers, connected clients and their sessions,
  their subscriptions, the cluster's nodes, and disconnecting a session. A tier that does not
  answer is reported with its reason rather than leaving a page empty.

- **Shipped** — NATS 2.x, over the official Go client and its jetstream subpackage. The
  second family with no administrative plane of its own, and the one with the most that can be
  missing: four tiers are probed when a connection opens - the protocol, JetStream, the server's
  HTTP monitoring endpoint, and the system account - and each of the last three can be absent
  for two different reasons that lead somewhere different, so six degraded reasons are reported
  rather than one. JetStream streams with their subjects, retention, storage and replica set;
  push and pull consumers; browsing and following by sequence; publishing on a subject, with a
  request that waits for a reply; purge by count, sequence or subject; the cluster's servers and
  their effective settings; client connections and disconnecting one; and the accounts, with
  their JetStream usage against the caps they were given.

  Core NATS is the part that does not fit any existing page, and it gets its own. A subject
  keeps nothing: a message is delivered to whoever is listening at that moment and is then gone,
  so there is no history to page back through and the subjects workbench is a live view rather
  than the message browser with a filter on it.

  Four things it deliberately does not have. There is no dead-letter page: a consumer that
  exhausts max_deliver stops redelivering and publishes an advisory, and the message is moved
  nowhere. There is no offset reset: a consumer's start position is fixed when it is created and
  the API refuses to change it, so the only way to move one is to delete it and make another -
  which changes its identity. There is no pending-entries list, because JetStream reports how
  many deliveries are unacknowledged and nothing about which. And accounts are read-only: no
  NATS server has a request that creates one, in configuration mode or in operator mode.

- **Shipped** — ActiveMQ Classic 5.x / 6.x and ActiveMQ Artemis 2.x, over Jolokia, the
  JMX-over-HTTP agent both brokers ship under their web console. One MQKind covers two products
  because they are one family to a user, and they share nothing a driver reads: different agent
  path, different MBean domain, different ObjectName keys, different attribute names, and browse
  results whose map keys do not overlap at all. Which one answered is settled when the connection
  opens, read off the MBean domain that responded to a search.

  The management plane is also the data plane here, which is what makes this driver unusual.
  Browsing and sending are JMX operations on both products, and browsing consumes nothing - so the
  message board carries no requeue caveat the way RabbitMQ's does, and every page works against a
  broker with every wire acceptor switched off. Queues and topics with their depth, counters and
  settings; durable subscriptions on either product, created and removed; sending with JMS headers,
  properties and a priority; dead letters found by walking the declarations backwards and retried
  back to the destinations they failed on - the first retry any family here has had; the broker
  with its store, journal and effective settings, and the brokers it bridges to; and client
  connections with the protocol each speaks.

  AMQP 1.0 is an optional tier, probed at connect time, for the one thing JMX cannot do: follow a
  destination as messages arrive, because the management plane is request/response and has no push.
  Topics only, and that is a safety rule - a JMS consumer consumes, so attaching one to a queue
  would take its messages. A broker with the acceptor off keeps every other page and says which of
  the three states it is in.

  Four things it deliberately does not have. There is no delayed delivery, and both products have
  it: the annotations must be a Long and both send operations take Map<String,String>, so a delay
  set through Jolokia is accepted, ignored, and would be reported as having worked. There are no
  offsets and no partitions, because JMS has neither - a message is acknowledged and gone, and
  nothing splits a destination in a way a consumer addresses. There is no access page: both
  configure authentication in XML read at startup, and JMX offers no operation that creates a user.
  And Classic's browse stops at maxBrowsePageSize, 400 by default, however deep the destination is;
  the limit is not readable over JMX, so the page reports the cap as a caveat rather than
  pretending the queue is 400 deep.

- **Shipped** — NSQ 1.x over the HTTP APIs of nsqd and nsqlookupd. There is no admin protocol:
  everything an operator can ask is a call on the same daemons that carry the messages, so this
  driver needs no wire client at all. Topics with the depth they hold, split between the topic's
  own queue and its channels' and summed across every nsqd carrying them; channels, which are
  this family's consumer groups, with their backlog, in-flight, deferred and requeued counts;
  creating, emptying, pausing and deleting either, on every daemon at once; publishing to one
  named daemon, repeated or held back for a delivery time; the cluster's nsqd beside the
  nsqlookupd that tell consumers where to find them; and who is connected, in both roles nsqd
  reports them in - consumers with the ready count that says which of them has stopped asking
  for work, and producers with what each has published.

  Four things it deliberately does not have, and all four follow from one fact: nsqd hands a
  message to a consumer and stops holding it. There is no browse, because there is no stored log
  behind a depth and no call that reads one back. There is no message id, because one exists only
  on the wire between nsqd and the consumer holding it. There are no dead letters, because a
  message requeued past its limit is dropped rather than moved. And there is no offset, so a
  channel's backlog can be consumed or emptied and moved no other way.

  One client is invisible and no page can fix it: anything publishing over HTTP. /pub is a
  request rather than a connection, so nsqd has nothing left to list once it has answered - only
  a producer holding a connection over the wire protocol appears.

  Three more absences are about what nsqd reports rather than what it stores. No rate of any
  kind: it counts messages since it started and nothing else. No disk figure: a topic's overflow
  file sits wherever --data-path points and the daemon never looks at it. And no credentials on
  the connection form, because its HTTP API authenticates nobody - --auth-http-address delegates
  authorisation for clients arriving over the TCP protocol and never touches these endpoints.

  Two things the driver had to get right that no other family here tests. A delete has to reach
  nsqlookupd as well as nsqd: the daemons forget a deleted topic and the directory does not, so a
  delete that stopped at nsqd leaves the name where a consumer looking it up still finds it. And
  emptying a topic has to empty its channels: nsqd copies each message into every channel as it
  arrives, so /topic/empty on its own answers 200 and moves nothing a page can see.


- **Shipped** — Amazon SQS through the AWS API. The first family with no broker address: there
  is nothing to dial, so a connection is a region and a credential, and the driver's own
  descriptor declares no endpoint field at all - which is what phase 11 built the seam for.
  Queues with what they hold split three ways; creating, editing, purging and deleting them,
  standard or FIFO; browsing; sending with named attributes, a delay, a repeat, and the group
  and deduplication ids a FIFO queue requires; and dead letters found by walking every queue's
  redrive policy backwards.

  FIFO queues are covered rather than deferred. The .fifo suffix changes ordering,
  deduplication and what a send must carry, and none of that changes how a queue is listed,
  created, emptied, deleted or read - so the difference lives in three form fields and two
  refusals, not in a second set of pages.

  The browse carries a caveat, and it is the honest one. SQS has a single read -
  ReceiveMessage - and it is the same call a consumer makes: what it returns is hidden from
  everyone else for the visibility timeout, and its receive count goes up permanently, which is
  what a redrive policy compares against. The driver hands every message straight back with a
  visibility timeout of zero, so the window is about as long as the request; it cannot close it,
  and it cannot undo the count. Hence a caveat rather than a silent best effort.

  What it deliberately does not have is mostly one fact and one boundary. SQS has no
  subscription of any kind - a consumer is whoever calls ReceiveMessage, and the service keeps
  no record of who that was - so there is no consumers page, no lag, no offset and no consumer
  count anywhere; a queue reports its subscribers as unknown rather than as zero, because zero
  would be a claim the service cannot support. And AWS runs the service, so there is no cluster,
  no node, no disk figure and no rate: what SQS reports is per queue, and everything else lives
  in CloudWatch, which is a different API under a different permission.

  Two more absences worth naming. There is no message-by-id lookup: an id is assigned on send
  and echoed on receive, and nothing indexes one. And there is no access page: who may call what
  is IAM's, one service further out, and a page editing the queue policy alone would claim to
  control access it cannot see.

- **Shipped** — Google Cloud Pub/Sub through the Pub/Sub API. The second family with no
  broker address - a connection is a project and a Google credential - and the first whose
  objects come in two kinds. Topics with the count of what reads each; creating, editing and
  deleting them; subscriptions as objects in their own right, created, listed and deleted
  independently of the topic; browsing a subscription; publishing with attributes and an
  ordering key; restore points and both kinds of seek; and dead letters found by inverting
  every subscription's dead-letter policy.

  The split between the two objects is what this phase was for. A Pub/Sub topic holds nothing:
  a publish is fanned out to whatever subscriptions exist at that instant and discarded if none
  do, and the service reports success either way. So the topics board leads with a subscription
  count where every other family leads with a depth, and a topic with none is the fault the
  alerts page raises - it is the one failure with no other symptom, because a discarded message
  leaves no backlog behind it. The mirror image gets a rule of its own: deleting a topic does
  not delete its subscriptions, and one left pointing at `_deleted-topic_` holds what it had,
  is billed for it, and will never receive again.

  The backlog is the one figure this family cannot report, and it is degraded with a reason
  rather than invented. `num_undelivered_messages` is a Cloud Monitoring metric: it is not a
  field on the subscription the admin API returns, and no call anywhere in Pub/Sub reports it.
  The only way to produce a number would be to pull the backlog and count it, which would
  deliver every message counted.

  The browse carries a caveat for the same reason SQS's does. Pull is the only read there is
  and it is the call a consumer makes: what it returns is held away from every other reader for
  the subscription's ack deadline, and its delivery attempt goes up - which is what a
  dead-letter policy compares against, so a message browsed often enough is dead-lettered with
  nothing having failed. The driver hands every message straight back and cannot undo the
  attempt.

  Seek is declared both ways, which took a correction. Moving to a moment and moving to a named
  snapshot are different gestures with different guarantees, and the emulator refuses the first
  one only for a subscription created with message ordering on - one subscription's setting
  rather than anything about the endpoint. So both capabilities stay declared and the refusal is
  an error at the call that names ordering.

  What it deliberately does not have follows from Google running the service: no cluster, no
  node, no disk figure and no rate - the rates are Cloud Monitoring's, like the backlog. There
  is no access page, because who may call what is IAM's, one service further out. And there is
  no message-by-id lookup: an id is assigned on publish and echoed on delivery, and nothing
  indexes one.

- **Shipped** — Azure Service Bus through the Azure SDK. The third hosted family and the
  first of the three that is reached by dialling something: a region and a project are not
  addresses, and a namespace is - both halves of this driver dial it, AMQP for the messages and
  Atom over HTTPS for the topology. Queues and topics on one board, subscriptions on another,
  rules on the routing page, a browse that takes nothing, a send that carries what a rule
  selects on, and the dead letters every entity is created with.

  The messages page is what this phase was ordered here for. Peek is non-destructive in a way
  neither hosted family before it could manage: it takes no lock, moves nothing, changes no
  delivery count, and a consumer running at the same moment misses nothing. SQS's read hides
  what it returns for a visibility timeout and Pub/Sub's raises a delivery attempt that counts
  towards being dead-lettered, so both had to warn. This one does not, and the absence is
  asserted in Go and in the renderer rather than left to be noticed - the way it would be lost
  is silent, because swapping the peek for a receive would still return messages.

  It also shows more than a consumer would ever be offered. A scheduled message is held back
  until its enqueue time and a deferred one has been set aside by sequence number; neither is
  handed to any receiver, and both appear on the page with a state saying which.

  Rules are the second concept, and they earn the routing page. Every other family keeps its
  filtering on the reader, where it is a field: a Pub/Sub subscription carries a filter string
  fixed at creation, an SQS queue carries none. A Service Bus rule is an object with a name -
  several may sit on one subscription, each a SQL or correlation filter plus an optional action
  that rewrites the message on the way in - so which messages reach which subscription is a
  topology, and it maps onto the page RabbitMQ used to have to itself. An exchange is a topic,
  a binding is a rule, and the binding's properties key is the rule's name, which is what a
  delete takes. What does not map is refused rather than approximated: a topic has no exchange
  type, and accepting one would let a form claim a fanout topic that does not exist.

  Dead letters are the third, and they settled a capability question. `CapDLQ` is a per-entity
  store the broker names and fills; `CapDeadLetterTopology` is an ordinary object something
  else points at, found by walking every object's configuration backwards. SQS and Pub/Sub are
  both the second. A `$DeadLetterQueue` is the first: every queue and every subscription is
  created with one, it is reached by suffixing the entity's own path, and it cannot be listed,
  sent to, renamed or shared. So this page can only be empty when the namespace is.

  What it deliberately does not have follows from Microsoft running the service: no cluster, no
  node, no disk figure and no rate - the rates are Azure Monitor's. Nothing registers as a
  consumer, so there is no reader count anywhere. And there is no message-by-id lookup: a
  message id is the sender's own field, nothing indexes it, and what addresses a message is the
  sequence number a browse resumes from.

  Two things the emulator could not exercise, recorded rather than stepped around. It reports
  no readable message count on any entity, so every depth and backlog is a dash there and the
  backlog capability is degraded for that endpoint alone - a real namespace answers it. And it
  refuses `ForwardDeadLetteredMessagesTo` unless the target is an absolute URI, where the real
  service takes an entity name. Both have live tests that fail if the emulator ever changes.

- **Shipped** — Amazon Kinesis Data Streams through the AWS API. The fourth hosted family and
  the second AWS one, reached the same way SQS is: a region and a signed request, with no
  address anywhere on the form.

  It is the family the canonical model pushed back on, and the shard is why. `Destination`
  offers `Partitions`, an int, and a stream really is divided into N shards taking writes - so
  the count is true and it is what the streams board shows. It is also the only true thing a
  number can say. A shard has an id, owns the slice of the 128-bit hash space that decides
  which records land on it, has a read quota of its own, and is changed by being split in two
  or merged with a neighbour rather than resized - which leaves the old shard in place, closed,
  still holding its records until retention expires, and named as its children's parent. So the
  detail got a port and a page of its own, `ShardInspector` behind `CapShards`, following Redis
  Stream's precedent of adding ports where the shared vocabulary has no home. `CapPartitions`
  is deliberately not declared: it is answered by `DestinationStats`, whose page is a read
  range per partition number, and a shard put through it would lose every field above.

  Two predictions the estimate got wrong, both about what browsing costs. Reading takes
  nothing: `GetRecords` removes no record, hides none and marks none, and any number of readers
  can read the same one until retention expires - so the caveat SQS and Pub/Sub carry, that a
  browse takes a message away from a consumer, is simply false here and a conformance test
  names their keys as the ones it must not be. What is true instead is a different consequence
  and gets a caveat of its own: a shard allows five `GetRecords` a second and two megabytes a
  second, shared with every classic consumer reading it, so a browse can throttle a running
  application without having taken anything from it. The driver's per-shard budget is five
  calls for that reason rather than as a tuning choice.

  The consumers page is enhanced fan-out and only that. A registration is a real object with a
  name and an ARN, created and removed on its own, and it is the only reader a stream knows
  about - everything else that reads one registers nothing and keeps its position in a DynamoDB
  table this connection never sees. So the backlog is degraded with a reason rather than filled
  in, and unlike Pub/Sub's the number does not exist in a second API either. `MillisBehindLatest`
  on a read is this app's own lag and is used for nothing but deciding when a shard has been
  read to its end.

  What it deliberately does not have follows from AWS running the service and from nothing ever
  being moved aside: no cluster, no node, no disk figure and no rate - the rates are
  CloudWatch's; no dead-letter page of either kind, because a record stays where it was written
  until retention expires whether it was read or not; and no purge or trim, because retention
  is the only thing that removes a record and it is a setting on the stream.

  Two alerts the other hosted families have are absent for a reason worth separating from
  "there is nothing to read": `topicUnsubscribed` and `queueNoConsumer` would fire on nearly
  every healthy stream here, because the ordinary way to read one registers no consumer at all.

  One thing LocalStack could not exercise, recorded rather than stepped around. It enforces
  neither half of the per-shard read quota - twenty calls in a quarter of a second all succeed
  against it - so the caveat's mechanism is asserted through the driver's own call budget, and
  a live test says so and fails if the emulator ever starts enforcing it.

- **Shipped** — IBM MQ 9.x through the two REST interfaces the mqweb server hosts, and through no
  wire client at all. The obvious client for this family is cgo over IBM's native MQ libraries,
  which would put the redistributable client on the critical path of every build and every CI job;
  what is here instead is the standard library against `/ibmmq/rest/v1/admin` for objects and MQSC
  and `/ibmmq/rest/v1/messaging` for messages, which is the shape ActiveMQ already proved.

  It has an address, and that took deciding. A connection names a host, a port, a channel and a
  queue manager, and only the first two are dialled: the driver opens `https://host:9443` and
  everything else is a path on it, so the descriptor declares a required endpoint field and never
  opens 1414. The queue manager is part of that address rather than a scope - it is a separate
  process with its own storage, log and objects, nothing crosses between two of them, and there is
  no unscoped IBM MQ connection at all - so a second queue manager is a second connection, named on
  the form or discovered when the server fronts exactly one.

  Channels are the concept the canonical vocabulary had no room for, and they got a port and a page
  rather than a corner of an attribute map. A channel is not a client connection: it is a definition
  that exists with nothing connected, it decides whether an application may connect at all, one of
  them carries a running instance per connected client, and a message channel can sit in doubt over
  a batch with no client involved. So `model.Channel`, `driver.ChannelInspector` and
  `model.CapChannels`, read only for the reason `ShardInspector` is - stopping a server-connection
  channel disconnects every application using it, which is a different gesture with a different
  blast radius. The definitions come from MQSC rather than from the REST channel resource, because
  that resource returns no server-connection channel at all and its status parameter answers with an
  empty array on every channel.

  Two interfaces means two authorisations, which is the second thing this family taught. The mqweb
  server maps the administrative and messaging interfaces to two roles and a deployment may hold
  them on two accounts - IBM's own developer image does exactly that - so the form collects an
  optional second credential and the connection probes the messaging interface when it opens. A
  credential holding only the administrative role keeps every board except the two that touch
  messages, and those say why rather than disappearing.

  Browsing takes nothing, which no other family reached through a management API here can say: the
  depth is the same afterwards and the messages stay in order. The caveat it carries instead is its
  own - the server returns character data and nothing else, so a message stored in any other format
  is listed with its identifier and refused when opened, which is the ordinary state of every dead
  letter. Sending goes to a queue and only to a queue: the messaging interface has no topic resource
  at any version, so publishing needs an MQ client.

  Dead letters are `CapDeadLetterTopology` rather than `CapDLQ`. Nothing here is a dead-letter queue
  by nature; what makes one is the queue manager's DEADQ attribute or another queue's backout queue
  pointing at it, which is a walk backwards exactly as RabbitMQ's dead-letter exchange is.

  What it deliberately does not have: no cluster page, because an MQ cluster is a set of queue
  managers publishing to each other's repositories rather than nodes of this one; no rates and no
  storage figures, because the REST interfaces report neither; no offsets of any kind, because a
  queue is not a log; and no access page, because authority records are per object and per
  principal, which is a page of its own rather than a column.

  Two things this driver leaves out that the family has, recorded rather than dressed up as absences
  in the product: creating and deleting a subscription, which DEFINE SUB and DELETE SUB would do,
  and altering a queue, which ALTER would - each field worth changing has its own consequence for
  applications already connected, and one control writing them all is not the shape to offer.

- **Designed, not yet implemented** — the one family below.

## Delivery order

| Phase | Scope | Done when |
| --- | --- | --- |
| 0–3 | The driver seam itself: contracts, backend ports, storage and bridge, frontend registry | RocketMQ behaves exactly as before, screen for screen |
| 4 | **RabbitMQ** | Done. An Exchanges/Bindings page exists and no offset concept leaks into the UI |
| 5 | **Kafka** | Done. Topics, consumer groups, lag, browse and publish work end to end, alongside quotas, reassignment and transactions, and no rate or dead-letter page pretends to exist |
| 6 | **Redis Stream** | Done. Streams, groups, browse, publish, the pending entries list, the server and its cluster, clients and ACL users all read a real broker, and no maxlen or message rate pretends to exist. Additive as predicted, with four new ports: a log's trim, a subscription's position, an entry publish, and the pending list |
| 7 | **Pulsar** | Done. Topics, namespaces and the tenants above them, subscriptions and cursors, browse and tail, a send console, dead letters and role grants all work end to end, and no page pretends to a tag, a disk figure or a user directory this family does not have |
| 8 | **MQTT** | Done. The first family with no admin plane of its own: what it can do is probed at connect time in three tiers — the protocol, the $SYS tree, and the broker's own REST API — and a tier that does not answer says why rather than going quiet |
| 9 | **NATS** | Done. Additive as predicted - no canonical page changed shape - but the first family whose driver reads the profile's auth mechanism rather than only its secrets, which is what found a dial-time bug that reset that mechanism on every family but RocketMQ |
| 10 | **ActiveMQ / Artemis** | Done. JMS semantics fit the canonical pages everywhere except where those pages assume a log: no offsets, no partitions, no trim. What they gained is a dead-letter page that is finally full, and the first retry in the app |
| 11 | **Connection shape** — the seam, not a driver | Done. A family can declare it needs no address, and the connection form, the profile store and the probe path all honour that. The driver's own descriptor is what says so |
| 12 | **NSQ** | Done. Topics and channels, with no message history and therefore no browse - confirmed. The management plane is the HTTP API of the daemons themselves, so the driver needs no wire client; what it had to get right instead is that every figure is a sum across daemons that each know only their own, that a delete has to reach the discovery tier, and that emptying a topic has to empty its channels |
| 13 | **Amazon SQS** | Done. The seam phase 11 built, used: a descriptor with no endpoint field, a profile that saves with an empty `Endpoints`, and a connection row that shows the region where every other family shows an address. Confirmed: no subscriptions, so no consumers page and no lag; no cluster, because AWS runs the service; and one read, which is the one a consumer makes - so the browse carries a caveat rather than pretending to be non-destructive |
| 14 | **Google Cloud Pub/Sub** | Done. Subscriptions are objects in their own right rather than a reader's position - created, listed and deleted independently of the topic, and carrying the whole of the delivery configuration. Their backlog does not map onto lag after all: `num_undelivered_messages` is a Cloud Monitoring metric and no call in the Pub/Sub API reports it, so the capability is degraded with a reason rather than filled in with a number that would have to be produced by pulling the backlog to count it |
| 15 | **Azure Service Bus** | Done. Peek is non-destructive, so this is the one messages page in the app with no caveat at all - and the absence is asserted rather than left to be noticed. Subscription rules did reach the routing page: a rule is an object with a name rather than a field on the reader, so an exchange maps onto a topic and a binding onto a rule. Two the estimate missed: the dead letters are a per-entity store the broker creates rather than a topology to walk, and a peek reaches scheduled and deferred messages no consumer is ever offered |
| 16 | **Amazon Kinesis** | Done. Shards are not partitions, and they got a port and a page rather than columns: a shard is named, owns a slice of the hash space, and is split and merged rather than resized - so a resize leaves a closed parent still holding its records. Two the estimate got wrong: browsing takes nothing at all, which no other hosted family here can say, and what it does spend is the shard's read allowance, so the caveat says that instead; and the backlog does not exist anywhere, not even in a second API, because a classic consumer's position lives in DynamoDB and a registered one has none |
| 17 | **IBM MQ** | Done. Channels are first-class and have no counterpart among the canonical pages, so they got a port and a page of their own - read only, because stopping one disconnects every application using it. Three the estimate missed: the family has an address after all, and it is the mqweb server rather than the queue manager's listener; the two REST interfaces authorise against two roles, so a credential can reach every board and no message; and the channel resource returns no server-connection channel at all, so the definitions come from MQSC. One it got right in an unexpected direction: browsing takes nothing, and the caveat is about format instead - the server returns character data only, so every dead letter is listed and none can be opened |
| 18 | **Solace PubSub+** | Done, and the estimate was right: a Message VPN is a scope selector, and it needed no new port - CapConnectionScope and a ScopeOption were the whole of it. Two the estimate missed, and both are the same mistake in different places: the field that reads as a queue's depth is a lifetime statistic that clearStats zeroes on a full queue, and the Message VPN reports its spool usage in bytes beside a quota in megabytes - so the two figures an operator opens this app for are the two SEMP invites you to get wrong. One it got right in an unexpected direction: browsing takes nothing at all, and the caveat is about what comes back instead - there is no message payload anywhere in SEMP, at any version |

Two ordering decisions worth keeping in view.

**RabbitMQ came before Kafka on purpose.** Kafka is close enough to RocketMQ that it would
have passed even if the abstraction were wrong, so it could not validate anything. RabbitMQ
disagrees with RocketMQ about offsets, partitions and consumer groups, and that disagreement
was the real test.

Kafka then found the opposite kind of problem. Where RabbitMQ pushed on the canonical model,
Kafka pushed on the canonical model's *silences*: rates and disk percentages that every other
family reports and Kafka does not, and a dead-letter page the canonical page set assumes. The
answer in each case was to cut the column rather than to fill it in.

**The hosted tier changes the connection form, not just the driver.** Every family through
phase 12 is "an address plus optional credentials". The hosted tier is "a region plus a
credential, and no address at all" — the first connection where an empty `Endpoints` is
still valid. Phase 11 is where that gets settled, before the first hosted driver rather
than during it: the driver's own descriptor becomes what says whether a family needs an
address, and the form, the profile store and the probe path follow it.

## Per-driver scope

What each driver talks to, which canonical pages it lights up, and what it cannot offer.
The canonical pages are `Destinations`, `Subscriptions`, `Messages`, `Publish`, `Cluster`
and `Access`.

> Everything below RabbitMQ is a scope estimate read off each product's published
> management API. It is planning input, not verified behaviour — each row is confirmed
> when its driver is built.

### Self-hosted

| Driver | Management plane | Pages it lights up | Notable gaps |
| --- | --- | --- | --- |
| **RocketMQ** 4.x / 5.x | Admin API over the remoting protocol | All six | A Proxy endpoint answers far less than a NameServer; capabilities narrow on connect |
| **RabbitMQ** | HTTP management plugin, plus AMQP 0-9-1 for messages | All six, plus Exchanges/Bindings, Connections, Dead letters, Virtual hosts, Policies, Definitions, Replication | No offsets or partitions; no named consumer groups; no stable message id; browsing requeues what it read and carries a caveat; shovel, federation and the stream protocol are plugins and degrade with a reason when absent |
| **Kafka** | The Kafka protocol itself, through franz-go and kadm | All six, plus log directories and SCRAM users | Confirmed: browse is an offset-range fetch rather than random access, and a key search is a scan. ACLs degrade with a reason on a cluster with no authorizer. No rate of any kind is reported, and no disk percentage exists; there is no broker-side dead-letter queue |
| **Pulsar** | Admin REST API + the binary protocol | All six | Done. The tenant and namespace ended up as both: a scope selector on every page, and a page of their own, because a topic is addressed as tenant/namespace/name and the selector needs somewhere to get its options from |
| **ActiveMQ / Artemis** | Jolokia REST over JMX, plus AMQP 1.0 for following a topic | All six, plus Dead letters, Connections and a live topic view | Done. The two products' trees share no ObjectName, no attribute name and no message-map key, and which one answered is read off the MBean domain. Browsing and sending are management operations, so both take nothing off a destination and need no wire client - the optional AMQP tier exists only because JMX cannot push. Confirmed: no offsets, no partitions and no trim, because JMS has none; no delayed delivery, because both send operations take Map<String,String> and the scheduling annotation must be a Long; no access page, because both configure authentication in XML; and Classic's browse stops at maxBrowsePageSize, which is not readable over JMX and is reported as a caveat |
| **Redis Stream** | The Redis protocol itself, through go-redis | All six, plus Pending entries, Clients and ACL users | Confirmed: no per-destination access control - the key patterns are on the user. The prediction that there would be no cluster topology was wrong: `CLUSTER NODES` answers it, and the driver reads every master and replica. A stream has no partitions, nothing about it is editable, and there is no dead-letter queue - what replaces it is the pending entries list, which is delivery records rather than messages. No message rate and no disk figure are reported anywhere |
| **NATS** | JetStream API, the server monitoring endpoints and the $SYS account | Destinations, Subscriptions, Messages, Publish, Subjects, Cluster, Connections, Accounts, Alerts | Done. Four tiers, each probed on connect: without JetStream the endpoint drops to publish and subscribe, and the cluster pages need either the monitoring endpoint or the system account - the monitoring endpoint answers for one server, $SYS for all of them |
| **NSQ** | nsqd and nsqlookupd HTTP APIs | Destinations, Subscriptions, Publish, Cluster, Connections, Alerts | Done. Confirmed: no message history, so no browse and no message id; no dead letters, because a message past its retry limit is dropped rather than moved; no offsets, so a backlog is consumed or emptied and moved no other way; no rate and no disk figure anywhere; and no credentials, because nsqd's HTTP API authenticates nobody. nsqlookupd is optional and degrades with a reason when a profile names none |
| **MQTT** | None in the protocol. Probed at connect time: the $SYS tree, and the broker's own REST API where it has one | Overview, Topics, Subscribe, Publish, Clients, Cluster, Alerts | No consumer groups, no offsets and no stored history — a message exists while it is in flight and is gone if nobody was subscribed. Topics are those holding a retained value, because nothing else is enumerable. Clients need a management API, which Mosquitto does not have |

### Hosted

| Driver | Management plane | Pages it lights up | Notable gaps |
| --- | --- | --- | --- |
| **Amazon SQS** | SQS API | Destinations, Messages, Publish, Dead letters, Alerts | Done. Confirmed: no consumer groups and no cluster, and receiving starts a visibility timeout so browsing carries a caveat. Two the estimate missed: the receive also raises the message's receive count, which a redrive policy compares against, so browsing can dead-letter a message with nothing having failed; and the dead-letter page is answerable after all, by walking every queue's redrive policy backwards |
| **Google Cloud Pub/Sub** | Publisher and Subscriber admin APIs | Overview, Destinations, Subscriptions, Messages, Publish, Dead letters, Alerts | Done. Confirmed: subscriptions are real objects, and pulling consumes - so the browse carries a caveat. The estimate got the backlog wrong: it does not map onto lag at all, because `num_undelivered_messages` is a Cloud Monitoring metric rather than a field on the subscription, so the capability is degraded with a reason. Two the estimate missed: a topic holds nothing, so a publish to one with no subscription is accepted and discarded with no symptom anywhere; and a subscription outlives the topic it reads, which is a leak nothing else would show |
| **Azure Service Bus** | Service Bus management API, and AMQP for the messages | Overview, Destinations, Subscriptions, Messages, Publish, Dead letters, Routing, Alerts | Done. Confirmed: no cluster, and peek is non-destructive - so the browse carries no caveat, which no other family here can say. Three the estimate missed: the management plane and the data plane are two different protocols on two different ports; the dead letters are a sub-entity of every queue and subscription rather than a topology to walk; and a peek reaches scheduled and deferred messages, which no consumer is offered at all |
| **Amazon Kinesis** | Kinesis API | Overview, Destinations, Shards, Subscriptions, Messages, Publish, Alerts | Done. Confirmed: no cluster, and shards are not partitions - but they needed a port and a page rather than a column set, because a listing has to carry closed shards and their lineage. Three the estimate missed: browsing takes nothing, so the caveat is about the shard's shared read allowance instead; a record has no id, so what addresses one is its shard and sequence number together; and the only readers the service knows about are the registered fan-out kind, which carry no position - so the backlog is degraded rather than reported |

### Enterprise

| Driver | Management plane | Pages it lights up | Notable gaps |
| --- | --- | --- | --- |
| **IBM MQ** | The administrative and messaging REST APIs the mqweb server hosts | Overview, Destinations, Channels, Subscriptions, Messages, Publish, Dead letters, Alerts | Done. Confirmed: channels are first-class with no canonical equivalent, and they got a port and a page. Three the estimate missed: the address is the web server rather than the queue manager; the two interfaces authorise separately, so the messaging half is a tier probed on connect; and the messaging interface carries character data only, so a dead letter is listed and cannot be opened. No cluster page - an MQ cluster is a set of queue managers rather than nodes of one - and no rate, storage figure or offset anywhere |
| **Solace PubSub+** | SEMP v2, and the REST messaging interface on its own port | Overview, Destinations, Routing, Messages, Publish, Clients, Dead messages, Broker, Alerts | Done. Confirmed: a Message VPN is a scope and needed no new port. Three the estimate missed: a queue's depth is not a field but the count on its message collection, because spooledMsgCount is a lifetime statistic; SEMP carries no message payload, so a browse that costs nothing still cannot show a body; and every endpoint ships pointing at a dead message queue no broker creates, which makes silent discarding the default state. No consumer groups - the product has none - and no cluster page beyond the one broker |

## Covered by an existing driver

Wire-compatible systems do not get a driver of their own. They connect through the driver
for the protocol they speak, and that driver narrows its capabilities on connect to what
the endpoint actually answers.

| Connect as | Systems |
| --- | --- |
| Kafka | Redpanda, AutoMQ, WarpStream, Confluent, Amazon MSK, Azure Event Hubs |
| MQTT | EMQX, Mosquitto, HiveMQ, VerneMQ |
| ActiveMQ or RabbitMQ | Amazon MQ |
| RocketMQ | Alibaba Cloud and Tencent Cloud RocketMQ |

## Out of scope

- **ZeroMQ, nanomsg** — no broker, and therefore no management plane to show.
- **Celery, Sidekiq, BullMQ** — application-level job queues layered on Redis or RabbitMQ.
  Inspecting them is a different product, not another driver.

## Beyond drivers

- Restore end-to-end UI coverage. The Playwright suite drove Electron through its CDP
  endpoint and was removed with it; the platform WebViews offer no equivalent on macOS.
  Options worth evaluating: driving the Linux WebKitGTK build in CI, or covering the same
  flows as Go integration tests against the `tests/e2e/rocketmq` environment.
- Dedicated UI for update download progress
- Broader RocketMQ 5.x Proxy and ACL management features
