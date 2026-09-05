# Changelog

All notable changes to MQ Studio are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

[简体中文](CHANGELOG.zh-CN.md)

## [Unreleased]

## [0.0.9] - 2026-09-06

ActiveMQ is the eighth driver, and the first family here that is two products
rather than one. Classic 5.x / 6.x and Artemis 2.x share a name and nothing
else underneath, and which of them answered when the connection opened decides
every call made after it.

Alongside it, a RocketMQ namespace can be picked from the sidebar rather than
carried on every name by hand, and a driver setting changed while a connection
was open now redials instead of going on with the old one in silence.

### Added

- NSQ is the ninth driver, and the first with no admin protocol at all:
  everything an operator can ask is an HTTP call on the same daemons that carry
  the messages, so it needs no wire client. A connection is a set of nsqd
  addresses rather than one endpoint, because a topic lives on the daemon it
  was created on and every figure the app shows is a sum across the set.

  Topics with the depth they hold, split between the topic's own queue and its
  channels'; channels, which are this family's consumer groups, with their
  backlog, in-flight, deferred and requeued counts; creating, emptying, pausing
  and deleting either, on every daemon at once; publishing to one named daemon,
  repeated or held back for a delivery time; the cluster's nsqd beside the
  nsqlookupd that tell consumers where to find them, with a warning when the
  two disagree about an address; and the connected consumers, with the ready
  count that says which of them has stopped asking for work.

  There is no message board and no dead letters, and both follow from one fact:
  nsqd hands a message to a consumer and stops holding it. There is no stored
  log behind a depth, no id anything indexes, and a message requeued past its
  limit is dropped rather than moved aside. There is no offset either, so a
  channel's backlog is consumed or emptied and moved no other way.

  Two things the daemons do that the app had to be told about. A delete has to
  reach nsqlookupd as well: the daemons forget a deleted topic and the
  directory does not, so a delete that stopped at nsqd leaves the name where a
  consumer looking it up still finds it. And emptying a topic has to empty its
  channels: nsqd copies each message into every channel as it arrives, so
  emptying the topic alone answers 200 and moves nothing anyone can see.

- ActiveMQ is the eighth driver, and one connection type covers both products:
  Classic 5.x / 6.x and Artemis 2.x are told apart when the connection opens,
  by which MBean domain answers. They share nothing underneath — different
  Jolokia path, different ObjectName keys, different attribute names, and
  browse results whose keys do not overlap — and none of that reaches a page.

  Queues and topics on one board, with their depth, counters and settings;
  durable subscriptions on either product, created and removed; browsing that
  takes nothing off the destination, because it is a management operation on
  both; sending with JMS headers, properties and a priority; dead letters found
  by walking the declarations backwards, and retried back to the destinations
  they failed on; the broker with its store, journal and effective settings,
  and the brokers it bridges to; client connections with the protocol each
  speaks, and disconnecting one. Where the broker's AMQP acceptor is reachable,
  a topic can also be watched live — the one thing the management plane cannot
  do, because JMX has no push.

  Two absences are the broker's rather than the app's. There is no delayed
  delivery: both products have it, and both send operations take a string map
  while the scheduling annotation must be a Long, so a delay would be accepted
  and silently ignored. And a Classic browse stops at 400 messages however deep
  the destination is, which the page reports rather than hiding.

- A RocketMQ namespace can be switched from the sidebar. It sits above the
  pages because it scopes all of them — a namespace is a prefix the client puts
  on every topic and group it names, so changing it re-points the whole tab —
  and picking one stores it on the connection and dials again, which means the
  tab reopens where it was left. The list is read off the cluster's own names,
  since RocketMQ keeps no namespace registry, and a namespace nothing carries
  yet can be typed in. (#78)

### Fixed

- Editing a connection's driver settings — a RocketMQ namespace, a Kafka
  security protocol — while it was connected saved the change and went on
  reading with the old one until the app was restarted, with nothing on screen
  saying so. Only the endpoints, the timeout and the credentials were being
  watched for a redial.

## [0.0.8] - 2026-09-05

A single change, to where releases are stored. The fast mirror is a bucket that
serves more than this project, and a release was writing to its root — two
projects publishing the same tag would have collided there.

### Changed

- Packages on the fast mirror moved from `dl.amigoer.com/<tag>/` to
  `dl.amigoer.com/mq-studio/<tag>/`. The GitHub mirror is unchanged, and so is
  every published digest: this moves where the files are served from, not what
  they are.
- A 0.0.7 install reaches this release through GitHub rather than the fast
  mirror. The path it was built with points at the old location, and nothing a
  manifest says is allowed to redefine a mirror a build shipped with — that
  rule is what stops a mirror from redirecting an installed app, and the cost
  of it is one release spent moving over. From 0.0.8 both mirrors are used
  again.

## [0.0.7] - 2026-09-05

This release is about how an update reaches you. Everything went through
github.com — the check, the download, and the site's own buttons — so a network
that could not reach it could neither learn that an update existed nor install
one. A release is now described by a small manifest that several hosts serve,
and which host answers is measured on your own machine rather than decided in
advance.

The Windows installer also never ran. Pressing install failed with an elevation
error, because the app started it with a call that cannot raise the permission
prompt. That fix cannot deliver itself: if you are on 0.0.6 or earlier on
Windows, install this one by hand.

### Added

- A release is mirrored. It is described by a manifest every mirror serves,
  with each package's digest written into it, and the app races the mirrors for
  that manifest — a check has to fetch it anyway, so the fetch doubles as the
  measurement of which host this network can reach. The winner is remembered
  for a week, so a routine check is one request and no race at all. If a mirror
  stops answering partway through a download the next one continues it, which
  is safe only because the digest travels in the manifest rather than beside
  the package: a mirror carries bytes without getting to say what they are.
  (#62)
- A manifest can name mirrors of its own, and those are merged into the list a
  build shipped with, so a mirror added to a future release reaches builds
  released before it existed. The merge only ever adds — nothing a mirror says
  can remove or redefine a path compiled into the app.
- The download buttons on the site carry a mirror link and a fallback, both
  resolved when the page is built. The site ships no JavaScript, so it cannot
  measure which one a visitor can reach; offering both is the most it can
  honestly do.

### Changed

- The update check reads that manifest instead of the GitHub releases API, and
  so does the site when it builds its download cards. The two can no longer
  disagree about what the latest release is.

### Fixed

- Installing an update on Windows failed with "The requested operation requires
  elevation" and never showed a permission prompt. The installer was started
  through a call that does not read a program's manifest and cannot elevate, so
  against an installer asking for administrator it did not prompt and did not
  run. (#77)
- Update failures showed the runtime's own error text with its class name in
  front, so "RuntimeError: failed to start the installer: fork/exec
  C:\Users\..." reached the reader exactly as written here.
- Failures whose meaning is fixed are now said in your language: a dismissed
  permission prompt, an install that cannot replace itself, a build too old to
  read the release index. Failures where the detail is the point — which mirror
  timed out and what it returned — are still shown as reported.
- The About page names all seven drivers. It listed six, leaving NATS out, and
  spelled Redis Stream as "Redis" — the one family list in the tree the seventh
  driver had left behind.

## [0.0.6] - 2026-09-03

NATS is the seventh driver, and the first family here whose answers come from
four separate places — the protocol itself, JetStream, the server's HTTP
monitoring endpoint, and the system account. All four are probed when a
connection opens, and every page says which of them it is reading, or why it
is empty.

Alongside it, three credential bugs that only a real broker could show: a
connection was dialled with its stored authentication mechanism reset to none,
RocketMQ's global access key was stamped onto connections of other families,
and RocketMQ's own key pair was never signed with at all.

### Added

- NATS is the seventh driver, over the official Go client and its jetstream
  subpackage. JetStream streams with their subjects, retention, storage and
  replica set; push and pull consumers with their pending, unacknowledged and
  redelivered counts; browsing and following a stream by sequence; publishing
  on a subject, with a request that waits for a reply; purge by count,
  sequence or subject, and deleting single messages; the cluster's servers
  with their routes and effective settings; client connections with what each
  is subscribed to, and disconnecting one; and the accounts, with their
  JetStream usage against the caps they were given.
- A subjects workbench for core NATS, which is the part that fits no existing
  page. A subject stores nothing: a message reaches whoever is listening at
  that moment and is then gone, so there is no history to page back through
  and the workbench is a live view rather than the message browser with a
  filter on it.
- Four tiers are probed when a NATS connection opens — the protocol,
  JetStream, the server's HTTP monitoring endpoint, and the system account —
  and each of the last three can be missing for two different reasons that
  lead somewhere different. A server built without JetStream is not an account
  denied it; a monitoring address nobody entered is not one that does not
  answer; credentials never given are not credentials refused. Six reasons are
  reported rather than one, because each sends the reader somewhere else.
- The cluster pages take whichever of the monitoring endpoint and the system
  account answered. The endpoint reports the one server whose port was named,
  and `$SYS` fans out to every server in the cluster, so each row records
  which source it came from and the page says when a figure covers one server
  out of several.
- Three alert rules NATS needs and no other family has: a stream whose Raft
  group has no leader, a stream with a replica behind, and a server that has
  disconnected clients for falling behind.
- Two authentication mechanisms: an nkey seed, and a credentials file signed
  by an operator.
- A RocketMQ connection can name a namespace, which scopes everything it does
  to that namespace: topics and consumer groups are listed under their short
  names, and every request the connection makes carries the wrapped ones. The
  field is in the connection form's advanced block, where a disabled "Instance
  ID" control had been drawn for it, and the namespace is shown beside the
  address in the connection list and the tab status bar so a reader can tell a
  scoped connection from an unscoped one. Leaving it empty is unchanged
  behaviour: the connection sees the cluster whole, raw names
  included. (#61, #63)

  This is the namespace RocketMQ 5.x actually implements — the client-side one,
  where `orders` goes on the wire as `ns%orders` and a consumer group's retry
  topic as `%RETRY%ns%GID`. The broker stores an ordinary topic and knows
  nothing about it, which is why it works on a stock cluster with no broker
  configuration. It is not `namespaceV2`: that sends two request-header fields
  no code in apache/rocketmq reads, so nothing here would honour them and
  nothing available could show them working.

### Fixed

- A connection is now dialled with the authentication mechanism it was stored
  with. Resolving a profile for dialling reset it to none on every family
  except RocketMQ, so a connection saved with a username and password
  authenticated as nobody. It went unnoticed because the drivers that had a
  mechanism until now read their credentials straight out of the stored secrets
  and ignore it.
- RocketMQ's global access key pair in settings is no longer stamped onto a
  connection of another family. It filled in any profile that carried no
  credentials of its own, which meant a NATS or Kafka connection was dialled
  with a mechanism its driver does not implement, using a different broker's
  credentials — and only for the users who had configured that pair.

- A failed connection test says why. The reason was computed and translated
  and then written into a `title` attribute, which the macOS webview draws no
  tooltip for, so the one button whose whole job is to produce it reported
  only that it had failed. It is text beside the button now. Two fields side
  by side also line up whatever their hints do — a hint that wrapped used to
  push its own input down and leave its neighbour's at the top — and the NATS
  hints, which were three times the length every other family uses, are one
  clause each.

- A RocketMQ connection's AccessKey and SecretKey are now actually sent. They
  were stored, decrypted and handed to the driver, and the client library then
  read neither - it had no signing code at all - so every admin call arrived
  unauthenticated. On a cluster with `aclEnable=true` that worked only where the
  broker's global whitelist happened to cover the caller, and the connection
  form's two credential fields, plus the global pair in settings that fills them
  in, made a promise nothing kept. Requests now carry the access key and an
  HMAC-SHA1 signature over the sorted header fields and the body, which is what
  the broker rebuilds and compares.

- The update dialog's exits point at the site rather than at GitHub Releases.
  The check-failed, install-failed and cannot-replace-itself buttons all
  opened the releases page, which is a dead end for anyone who cannot reach
  github.com. They open the site's download section now, and the dialog's
  standing link opens the changelog there, both following the language the app
  is running in. The check and the download themselves still go to GitHub, so
  this is only where the buttons land. (#62)

- Every launch now checks for a release, five seconds after the window comes
  up, instead of waiting out whatever was left of the twenty-four hour
  interval. An application that is opened and closed a few times a day never
  reached the end of one, so no background check ever ran and the only way to
  hear about a release was to press the button in settings. The interval is
  unchanged for a session that stays up. (#67)

- Skipping a version no longer disables the check button. "Skip this version"
  means stop announcing it, so a check the user presses for now takes the
  release back off the skip list and offers it: it used to be answered with
  "you are up to date" naming the version already running, and nothing
  anywhere could undo a skip. The settings card also says a release was
  skipped rather than going on announcing it beside a button offering to look
  for one. (#66)

- The update dialog's bottom corners are square no longer. The footer is the
  one bar with an opaque background, and clipping it to the dialog's radius is
  not enough on WebKit, which drops that clip on a transformed, animated box
  whose subtree scrolls -- which is every one of the dialog's own classes. It
  now carries the radius itself. (#53)

## [0.0.5] - 2026-09-02

Three drivers at once — Redis Stream, Pulsar and MQTT — which takes the count
to six. Each is written in its own family's vocabulary rather than as a
translation of another's: Redis keeps a pending entries list where the others
have a dead-letter queue, Pulsar carries properties where RocketMQ carries
tags, and MQTT has no administrative plane at all, so what a connection can do
is probed when it dials rather than declared up front.

### Added

- Redis Stream is the fourth driver. Streams with their length, memory and
  entry range; consumer groups with lag and every reposition XGROUP SETID
  offers; browsing entries by time window or by id, and writing them as the
  ordered field lists they are; the pending entries list with claim,
  auto-claim and acknowledge; the server's memory, persistence and slow log;
  its client connections; and ACL users with their key, channel and command
  rules. Standalone, sentinel and cluster all connect, and a cluster's streams
  are listed from every master rather than from the node that was dialled.

- Two figures the other drivers report are deliberately absent, because Redis
  does not have them. It counts commands rather than messages, so there is a
  command rate under its own heading and no message rate; and it reports
  memory rather than disk, so there is a memory meter and no disk percentage.
  A server with no maxmemory has no meter at all, since there is no cap to be
  a percentage of.

- The pending entries list replaces the dead-letter page for this family.
  Redis moves nothing aside and gives up on nothing: an entry handed to a
  consumer stays in the stream and stays owed to that consumer until it is
  acknowledged or claimed away. Acknowledging is confirmed as destructive
  because it quietly is - it settles the entry with nothing having processed
  it - and an auto-claim reports what it found already gone as well as what it
  moved, which is the only moment anything says work was lost rather than
  reassigned.

- Apache Pulsar, as the fifth driver. Topics with their partitions and storage
  kind, the namespaces and tenants above them, subscriptions with their
  backlog and cursor, a message browser and live tail, a send console, brokers,
  dead-letter topics, role grants and alerts. It speaks the binary protocol for
  data and the admin REST API for everything else, and reports the two
  separately: a connection whose web service answers while its broker port does
  not can still read every page, and says why it cannot publish.
- Pulsar's own vocabulary rather than a translation of another family's. There
  is no tag anywhere - what a RocketMQ producer puts in one, a Pulsar producer
  puts in a property, so the send console collects properties and the message
  browser filters on them. Subscriptions are stored cursors that exist without
  a consumer attached, so an idle one is a normal state rather than a group
  that has gone away. And the tokens page lists role grants rather than
  accounts, because Pulsar authorises the subject of a token and keeps no
  directory of them.
- A blocked-subscription alert, which no other family has. Past its
  unacknowledged limit the broker stops delivering to a subscription entirely -
  from the backlog alone that is indistinguishable from a slow consumer, and it
  is fixed by acknowledging or raising a limit rather than by touching the
  consumer.
- **MQTT 3.1.1 and 5.0.** The first broker family here with no administrative
  plane of its own, so what a connection can do is decided when it dials rather
  than declared up front, in three tiers: the protocol itself, the `$SYS` tree
  most brokers publish about themselves, and the REST management API EMQX and
  its peers add. A tier that does not answer is reported with its reason, so a
  page says why it is empty instead of looking broken — and the two are told
  apart, because "this broker refused the subscription" and "this broker
  publishes no such tree" are fixed in different places.
- Publishing with QoS, the retain flag and the MQTT 5.0 properties. The result
  says which kind of success it was: at QoS 0 nothing is acknowledged, at QoS 1
  and 2 the broker answers, and under 5.0 it can answer that it accepted the
  message and had nobody to give it to.
- A live subscribe workbench. It reports how many messages it had to drop and
  whether the session is still up, because a stream that is quietly losing and
  one that is quiet look identical otherwise, and so do a dropped connection
  and a silent broker.
- A topic list built from the broker's retained messages, which is the only
  thing MQTT can enumerate — the page says so, so a device publishing without
  the retain flag is not read as a device that has stopped publishing.
- Clients and sessions, their subscriptions, the cluster's nodes and
  disconnecting a session, where the broker offers a management API. A session
  that outlived its connection is marked: the broker keeps queueing for a
  client that is not there, and nothing on the device's side shows it.
- Alert rules of MQTT's own. It had been falling through to RocketMQ's, which
  read a broker ordinal, a commit log's disk usage and a dead-letter topic —
  three things MQTT never reports — so no MQTT connection could raise an alert
  at all.

### Fixed

- A degraded reason from a driver whose family name contains a hyphen was
  rendered as the raw key rather than as a sentence. No family had one until
  now.
- A degraded page's reason was invisible in every family. The sidebar entry
  carrying it was disabled, so it received no pointer events at all; the reason
  itself was a `title` attribute, which the macOS webview does not render; and a
  family with no reason strings of its own showed the key instead of a sentence.

### Notes

- Paho ships two Go libraries that do not overlap: one speaks MQTT 3.1.1 and
  the other 5.0, and a broker configured for either refuses the other's
  CONNECT. Both are used, and the protocol version is a field on the connection
  form rather than something negotiated.
- TCP, TLS, WebSocket and WebSocket over TLS are all supported.

### Known limitations

- macOS builds are not signed by a registered Apple developer. The disk image
  carries a First Run helper that clears the quarantine flag.

## [0.0.4] - 2026-09-01

The update flow, end to end. A release now announces itself on every launch
until it is answered, and the notice that announces it is also where the update
is taken.

### Added

- A pending release is announced once per launch, by a notice that stays up
  until it is acted on rather than clearing itself after a few seconds. The
  memory is the session's, so a release closed without an answer comes back the
  next time the app starts; skipping it is the one control that stops it for
  good. A check the user pressed opens the dialog instead of raising a notice,
  since they are already waiting on the answer.
- The update dialog carries the whole of an update - what changed, the download
  and its progress, and the restart - and opens from the title bar and from the
  notice. Installing no longer means a trip to the settings page, which was the
  only place that could do it. The release page stays reachable from every
  phase, because it is the way through when the app cannot replace itself.
- Release notes render as markdown rather than showing their own markers: bold,
  links, the rule, GitHub's `> [!IMPORTANT]` banner, and a bullet wrapped
  across source lines as one row rather than several. Links open in the system
  browser, since the webview has no way back. The renderer emits no HTML and
  degrades anything it does not recognise to a paragraph, because release notes
  are remote content.

### Changed

- The title bar's update control is an up arrow rather than a refresh glyph,
  and it leads where the update is: with a release pending it opens the dialog
  and names the version in its tooltip, and with nothing pending it starts a
  check as before.
- Nothing describes itself as a RocketMQ client any more. The macOS bundle, the
  Windows file description, the Linux desktop entry and package metadata, and
  the window description now say "message queues", which does not go stale as
  drivers arrive. The Linux keywords are where broker names belong, and they
  gain Kafka.

### Fixed

- The title bar icons rendered 22% larger than the size they were written at.
  They were expressed in `rem` against a base of 13, but the zoom ladder scales
  the document instead of setting a root font size, so the browser's 16 was
  what applied. The cluster is sized in pixels now, back to the 28px the 40px
  bar was built around.
- The search button stood taller than everything beside it and set its label a
  size above the tabs', because it used shadcn's page default in a bar that is
  not a page.
- The command palette was pinned 96px from the top, which sits high in a window
  at least 750px tall. It is centred now, at the cost of the input moving as
  results are filtered.
- The update notice stayed up after it had been answered: skipping a version
  left a notice still claiming that version was waiting, and the same notice
  sat through the download it had started.
- A Chinese sentence that wrapped straight after an emphasis marker gained a
  space in the middle of itself, because the join compared the marker rather
  than the character a reader sees there.

### Known limitations

- Pulsar, Redis Stream and MQTT appear in the interface and are disabled.
- macOS builds are not signed by a registered Apple developer. The disk image
  carries a First Run helper that clears the quarantine flag.

## [0.0.3] - 2026-08-31

Kafka support, over the Kafka protocol itself rather than a management API
beside it. Topics, consumer groups, records and access control, with the
figures Kafka does not report left out rather than filled in.

### Added

**Kafka 3.x and 4.x**

- Connect to a cluster over its own protocol, with SASL/PLAIN, SASL/SCRAM and
  TLS. The SCRAM digest is a field of its own because SHA-256 and SHA-512 are
  separate credentials on the broker: a user that exists under one fails under
  the other, and reporting that as a wrong password would be a lie.
- Overview, topics, consumer groups, messages, a send console, the cluster,
  access control, client quotas, and alerts.
- Topics with their partitions, leaders, in-sync replicas and offline replicas,
  their replication factor and minimum in-sync replicas, and the whole settings
  document behind a row. Create, alter and delete; an alter touches only the
  settings it is given, so nothing an operator never saw is reset to a default.
- Consumer groups with lag per partition and per member, their state, their
  coordinator and their assignor. All five of Kafka's offset resets - earliest,
  latest, a moment in time, an exact offset and a signed shift - plus copying
  one group's positions onto another and deleting a group.
- Browsing a log by the latest window, an offset range, a moment in time, or a
  key; and following the end of one. Browsing joins no consumer group and
  commits nothing, so it is safe to point at production.
- Producing with a key, headers, a pinned partition and a chosen
  acknowledgement level, and being told the partition and offset the record
  landed on.
- Brokers with their effective settings and their log directories, including
  which partitions are taking the space.
- ACLs grouped by principal, and the SCRAM users a cluster stores. Both degrade
  with a reason on a cluster running without an authorizer, rather than failing.
- Emptying a topic, which is Kafka's own truncation: every record becomes
  unreadable and the offsets do not restart, so a consumer sitting at 900 stays
  at 900 and is simply caught up.
- Moving a partition to different brokers, watching the move, and cancelling
  one in flight. A move has no completion event on Kafka - it is done when the
  partition stops reporting one - so the page reads the cluster rather than
  claiming it finished. Preferred-leader election alongside it, for putting the
  leadership back where the replica list says it belongs.
- Client quotas: the limits attached to who is calling rather than to what they
  are calling, for a user, an application or an address, and for the default
  each of them falls back to when no quota names them.
- Transactions on the cluster page: which transactional producers exist, what
  each is holding, how long it has held it, and whether it has outlived the
  timeout the coordinator undertook to abort it by. It is the only page that
  shows a pipeline stopped by a producer that died mid-transaction, because
  everything else about such a cluster reads healthy.
- Alerts from partition health - under-replicated, offline and leaderless
  counted separately - and from consumer group lag.

### Fixed

- Clearing a connection's credentials did not clear them. Only RocketMQ's
  access key pair was removed by name; every other family's password survived
  being cleared, and the next connect used one the form had reported as gone.
  RabbitMQ has had this since it shipped.
- The message shown when a connection has no address named a RocketMQ
  NameServer, which sent Kafka and RabbitMQ users looking for a field their
  form does not have.

### Notes

- Three things Kafka does not report, and this release does not invent. There
  is no dead-letter page: Kafka has no broker-side dead-letter queue, and the
  .DLT suffix belongs to Spring Kafka rather than to Kafka. There is no rate
  anywhere, because the admin protocol reports none. And there is no disk
  percentage, because Kafka reports the bytes its partitions occupy and nothing
  about the filesystem holding them.

## [0.0.2] - 2026-08-31

RabbitMQ support. The whole management plane, with messages carried over AMQP
rather than the management API's publish and get endpoints, so a send waits for
a publisher confirm and a browse behaves like a real consumer.

### Added

**RabbitMQ 3.x and 4.x**

- Connect to a broker's HTTP management plugin, with the AMQP data plane dialled
  alongside it. The connection identifies itself on the broker as
  `mq-studio: <name>`, so an operator can see which client is which.
- Overview, queues, exchanges and bindings, connections and channels, messages,
  dead letters, a send console, and nodes.
- Queues with their full arguments: classic, quorum and stream, durability,
  TTL, max length and overflow, dead-letter exchange and routing key, single
  active consumer. Declare, purge, move between queues, and delete.
- Exchanges and bindings: all four types plus alternate exchange, and bindings
  with their routing key and arguments, including a headers exchange's
  `x-match`.
- Browse over AMQP rather than the management API, filtered by routing key or
  header. The queue is left as it was found, and the page says what a browse
  costs: what it read comes back flagged redelivered.
- Publish with confirms: target exchange and routing key, mandatory,
  persistent, priority, expiration, headers, correlation id, reply-to and
  content type. A message nothing is bound to route is reported as unroutable
  rather than as a success.
- Dead letters read from the `x-death` header: which queue the message came
  from, why it was rejected, how many times, and when. Republish one or many
  back to their original queue or somewhere else, or drop them.
- Connections and channels with protocol, heartbeat, prefetch, unacknowledged
  count and flow-control state, and closing one with a reason.
- Nodes with their memory breakdown, resource alarms, partitions, the broker's
  own health checks, feature flags, and which deprecated features are actually
  in use.
- Virtual hosts: create, edit and delete, default queue type, deletion
  protection, tracing, and the connection and queue limits.
- Users and permissions: users and tags, the configure/write/read regex triple
  per virtual host, topic permissions, and per-user limits. Editing a user's
  tags no longer needs their password.
- Policies and operator policies with priority, pattern and definition, plus
  which policy a given queue actually matched; runtime and global parameters
  alongside them.
- Definitions: export the whole broker or one virtual host to a file, and
  import one after seeing what it will create.
- Shovels and federation: what exists, whether it is running, and the broker's
  own sentence when it is not. Read and delete only - a definition carries
  another broker's credentials, which are stripped before they leave the
  driver.
- Stream queues report the clients attached over the stream protocol, which
  never appear among a queue's AMQP consumers.
- Alerts derived from RabbitMQ's own figures: its resource alarms, network
  partitions, the approach to either watermark, a queue with a backlog, a queue
  with nobody reading it, and connections the broker is throttling.

### Fixed

- Alert rules read RocketMQ's attribute keys against every connection, so a
  RabbitMQ broker was measured for figures it never reports and raised nothing
  however badly it was doing.
- Management requests ignored the request timeout configured on the connection,
  because the underlying library takes no context. A slow broker could hold a
  page open indefinitely.
- A wrong password was reported as "enable the management plugin", sending the
  reader off to reconfigure a broker that was fine.
- Saving a connection kept only RocketMQ's access key pair and dropped every
  other credential, filing the connection as anonymous. Nothing could reach it
  in 0.0.1, where RocketMQ was the only driver, but it made a RabbitMQ
  connection impossible to save - and the form's test button passed, because it
  probes what was submitted rather than what was stored.

### Known limitations

- Kafka, Pulsar, NATS, MQTT and the rest appear in the interface and are
  disabled.
- Shovel, federation and the stream protocol are RabbitMQ plugins. A broker
  without them keeps the page, disabled, with the reason on it.
- macOS builds are not signed by a registered Apple developer. The disk image
  carries a First Run helper that clears the quarantine flag.

## [0.0.1] - 2026-08-31

First release of MQ Studio as a rebuilt project. MQ Studio is a desktop client
for message brokers, organised around a driver port so that support for a
broker family is one implementation behind a shared interface, rather than
assumptions spread through every page.

This release ships RocketMQ support only. The other protocols appear in the
interface and are disabled.

### Added

**Architecture**

- A driver port: a broker family is a driver behind one interface, with the
  services and the bridge above it agnostic to which family answered.
- A capability model: each connection reports what its endpoint can actually
  do, and the interface is drawn from that. A page the endpoint cannot serve is
  disabled with the reason; one the family has no concept of is not drawn.
- Multiple connections open at once, each in its own tab with its own pages,
  every request naming the connection it runs against.

**RocketMQ 4.x and 5.x**

- Overview, topics, consumer groups, message search, dead letters, publishing
  and cluster health, read from a live NameServer.
- Topic and consumer group operations: create, edit and delete topics; reset a
  group's read position by time; clone one group's positions onto another;
  write a single queue's offset.
- Message operations: query by key, tag, time window or message id; follow a
  topic live; trace where a message got to; resend a dead letter; hand one
  message to a named consumer and read back what its handler returned.
- Cluster operations: read a broker's and the name servers' effective settings,
  see how far replicas trail, run housekeeping on demand, take a broker out of
  the write path to drain it.
- Access control: RocketMQ 5.3 users and rules, readable and writable, with 4.x
  plain_acl kept as a write-only fallback that says so on the page.
- Alerts: broker offline, group offline, backlog, disk water level and dead
  letters, evaluated across every open connection, surfaced in the title bar
  and on a page of their own, with optional desktop notifications.

**Platforms**

- macOS on Apple Silicon and Intel, Windows on x64 and ARM64, and Linux on x64
  and ARM64 as .deb, .rpm and AppImage.

### Known limitations

- RocketMQ is the only broker that can be connected. The RabbitMQ driver is
  written but its pages are not built yet.
- Four operations are held back by defects in the RocketMQ admin library this
  application is built on, most visibly consumer group creation and editing.
  Each is pinned by a test asserting the current behaviour, so fixing the
  library is what unblocks them.
- macOS builds are not signed by a registered Apple developer. The disk image
  carries a First Run helper that clears the quarantine flag.

### Notes

- Version numbering starts over here. The 0.1.x builds predate the rebuild and
  have been removed.
