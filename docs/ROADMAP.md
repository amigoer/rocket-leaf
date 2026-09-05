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

- **Designed, not yet implemented** — the seven families below.

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
| 11 | **Connection shape** — the seam, not a driver | A family can declare it needs no address, and the connection form, the profile store and the probe path all honour that. The driver's own descriptor is what says so |
| 12 | **NSQ** | Topics and channels, with no message history and therefore no browse |
| 13 | **Amazon SQS** | Queues, messages and a send console, on a connection that carries a region and a credential rather than an address |
| 14 | **Google Cloud Pub/Sub** | Subscriptions are objects in their own right rather than a reader's position, and their backlog maps onto lag |
| 15 | **Azure Service Bus** | Peek is non-destructive, so this is the one hosted family whose messages page needs no caveat, and subscription rules reach the routing page |
| 16 | **Amazon Kinesis** | Shards are not partitions: they get their own columns instead of borrowing the canonical ones |
| 17 | **IBM MQ** | Channels are first-class and have no counterpart among the canonical pages, so they get a page of their own |
| 18 | **Solace PubSub+** | A Message VPN is a scope selector, the shape Pulsar's namespaces already proved |

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
phase 7 is "an address plus optional credentials". The hosted tier is "a region plus a
credential, and no address at all" — the first connection where an empty `Endpoints` is
still valid. Whether the schema-driven form can express that should be settled while the
page contract is being fixed, not in phase 8.

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
| **NSQ** | nsqd and nsqlookupd HTTP APIs | Destinations, Subscriptions, Publish, Cluster | No message history, so no browse |
| **MQTT** | None in the protocol. Probed at connect time: the $SYS tree, and the broker's own REST API where it has one | Overview, Topics, Subscribe, Publish, Clients, Cluster, Alerts | No consumer groups, no offsets and no stored history — a message exists while it is in flight and is gone if nobody was subscribed. Topics are those holding a retained value, because nothing else is enumerable. Clients need a management API, which Mosquitto does not have |

### Hosted

| Driver | Management plane | Pages it lights up | Notable gaps |
| --- | --- | --- | --- |
| **Amazon SQS** | SQS API | Destinations, Messages, Publish | No consumer groups and no cluster; receiving starts a visibility timeout, so browsing carries a caveat |
| **Google Cloud Pub/Sub** | Publisher and Subscriber admin APIs | Destinations, Subscriptions, Publish | Subscriptions are real objects and backlog maps cleanly to lag; pulling consumes, so browse needs a snapshot or a caveat |
| **Azure Service Bus** | Service Bus management API | Destinations, Subscriptions, Messages, Publish, plus rules on the routing page | No cluster; peek is non-destructive, so browse needs no caveat |
| **Amazon Kinesis** | Kinesis API | Destinations, Subscriptions, Messages, Publish | No cluster; shards are not partitions and need their own column set |

### Enterprise

| Driver | Management plane | Pages it lights up | Notable gaps |
| --- | --- | --- | --- |
| **IBM MQ** | Administrative REST API | All six | Channels are first-class with no canonical equivalent and are the likely override |
| **Solace PubSub+** | SEMP v2 | All six | Message VPN becomes a scope selector, as namespace does for Pulsar |

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
