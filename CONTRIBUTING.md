# Contributing to MQ Studio

[简体中文](CONTRIBUTING.zh-CN.md)

MQ Studio is one desktop client for every message queue, and it grows one
driver at a time. The most useful thing you can send is a report from a broker
nobody here runs — a version, a topology, a deployment that behaves differently
from the ones the tests cover. Code is welcome too, and this file is what you
need to know before writing any.

## Ways to help

- **Report a bug.** The three issue templates each exist in English and
  Chinese; pick either language.
- **Ask for a driver.** The [driver request](https://github.com/amigoer/mq-studio/issues/new?template=5-driver-request.yml)
  template asks one question that decides whether a driver is possible at all:
  what the app can reach from a desktop, and how.
- **Test a release candidate** against a cluster this project has no copy of.
- **Fix the translations.** Every user-facing string lives in
  `frontend/src/i18n/locales/en.json` and `zh.json`, and the second one is
  where a clumsy phrasing is most likely to survive unnoticed.
- **Write code.** Read the rest of this file first.

## Opening an issue

Blank issues are turned off, and the templates are why: nearly every question a
maintainer would have to ask is a field on the form. Fill in the version and
the broker even when the bug looks unrelated to either — that pair is what
decides whether it can be reproduced here at all.

Issues are read in both English and Chinese. Use whichever you write more
comfortably; the templates come in both.

## Labels

Three axes and a small workflow group. An issue normally carries one `type:`,
one `area:`, and a `driver:` when the report is family-specific.

- **`type:`** — `bug`, `feature`, `driver`, `docs`, `question`. The issue
  templates apply this one for you.
- **`area:`** — which part of the app: `connections`, `topics`, `messages`,
  `consumers`, `cluster`, `admin`, `app`, `i18n`, `website`, `ci`.
- **`driver:`** — the broker family, one per shipped driver.
- **workflow** — `needs:info`, `needs:repro`, `blocked:upstream`, and the
  familiar `good first issue`, `help wanted`, `duplicate`, `wontfix`.

The set lives in [`.github/labels.json`](.github/labels.json) rather than only
on GitHub, so it can be reviewed in a diff and rebuilt after someone deletes one
by hand:

```bash
npm run labels:sync              # print what differs
npm run labels:sync -- --apply   # write it
```

The dry run is the default because a rename and a prune both change what is
attached to issues people have already filed.

## Getting set up

You need Go (the version `go.mod` pins), Node.js 20.19+ or 22.12+, npm, and the
[Wails 3 CLI](https://v3.wails.io). Docker is needed only for the live tests.

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
make install
make dev
```

`make help` lists every target. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
has the process model and the repository layout.

## Before you push

```bash
make check
```

That is the same gate CI runs: version consistency across the tree, the
changelog references described below, the frontend build and type check,
`gofmt`, `go vet`, the Go and frontend unit tests, and a check that the
generated TypeScript bindings are not stale.

If you changed a Go service signature, regenerate the bindings and commit the
result:

```bash
npm run generate:bindings
```

## Live tests

Every driver is gated by tests that run against a real broker rather than a
mock. Locally they are opt-in, so a plain `go test ./...` stays offline and
quick even with every container running:

```bash
npm run e2e:kafka:up && npm run e2e:kafka:seed
MQ_STUDIO_E2E=1 go test ./internal/driver/kafka/...
```

Each family has its own `up`, `seed` and `down` scripts — `npm run` with no
arguments lists them. `npm run test:e2e` runs the whole live suite against
whatever is up.

In CI the opt-in variable is not consulted at all. The workflow starts every
environment, so a missing broker is a failure rather than a skip. Two rules
follow from that, and a new live test needs both:

- it calls `e2e.Require(t, e2e.Env{...})` with a probe **and** a `Family` — an
  `Env` without a family could not be claimed by any shard, so it would go
  unrun instead of red;
- that family has a shard in [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Commits

Commit messages follow [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)
and are written in English.

```
<type>(<scope>): <subject>

[body]

[footer]
```

- `type` is one of `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`,
  `perf`, `ci`, `revert`.
- `scope` is optional and lowercase — the driver, the layer, or the area:
  `kafka`, `update`, `shell`, `e2e`.
- `subject` is imperative, lowercase, no trailing period, 50 characters or
  fewer.
- The body explains *why*, wrapped at 72 columns. Skip it when the subject
  says everything.
- A breaking change takes a `!` after the type and a `BREAKING CHANGE:` footer.

Real examples from this repository:

```
fix(update): repair the update lifecycle end to end
feat(nats): add the NATS driver
ci: gate every change on the changelog naming its issues
```

## Branches

`<type>/<short-description>` in kebab-case, using the same types as the commits:

```
feat/rocketmq-namespace
fix/e2e-seed-silent-failure
ci/changelog-reference-gate
```

## The changelog rule

A commit that closes an issue carries the footer `Closes #NN` in its **body**.
Use `Refs #NN` when the issue deliberately stays open. `Fixes` and `Resolves`
are not recognised — this repository uses one spelling and a second would only
split it.

Every `Closes #NN` in the commits since the last tag must then be named in the
`Unreleased` section of **both** changelogs:

- `CHANGELOG.md`
- `CHANGELOG.zh-CN.md`

The **Changelog** check fails otherwise, and it names the numbers and the
commits each arrived on, so the section can be written straight from the
failure. Run it yourself with `npm run check:refs`.

Two things catch people out:

- A number inside backticks does not count. Code spans are matched first, on
  purpose: a bullet writing `` `#61` `` as a literal documents nothing.
- Both files, not either. A reference in English only is reported as a
  mismatch.

The rule exists because a reader only ever meets a change in the release notes,
and the commit footer is otherwise the only record of which issue asked for it.

## Things that come in pairs

Anything a user reads exists twice, and the second copy is the one that gets
forgotten:

| English | Chinese |
| --- | --- |
| `README.md` | `README.zh-CN.md` |
| `CHANGELOG.md` | `CHANGELOG.zh-CN.md` |
| `CONTRIBUTING.md` | `CONTRIBUTING.zh-CN.md` |
| `docs/INSTALL.md` | `docs/INSTALL.zh-CN.md` |
| `docs/ROADMAP.md` | `docs/ROADMAP.zh-CN.md` |
| `frontend/src/i18n/locales/en.json` | `frontend/src/i18n/locales/zh.json` |
| `website/src/i18n/en.ts` | `website/src/i18n/zh.ts` |
| `docs/images/hero-{light,dark}.svg` | `docs/images/hero-{light,dark}.zh-CN.svg` |

## Adding a driver

A driver is a package under `internal/driver/` that satisfies the port in
`internal/driver/driver.go` and declares its capabilities. The pages draw
themselves from those declarations, so a capability a driver claims and cannot
answer produces a page that looks broken rather than honest.

The part that is easy to get wrong is everything *outside* the driver package.
Two branches that each add a driver never conflict in a table — both rows
survive the merge — while the prose, the art and the counts beside them are
single lines only one side touched, and the merge keeps one version silently.
After adding a family, update all of these:

- `internal/model/mqkind.go` — the kind and its display name.
- `README.md` and `README.zh-CN.md` — the hero `alt` text, the sentence listing
  the drivers available today, the driver support table, and the roadmap table.
- `docs/ROADMAP.md` and `docs/ROADMAP.zh-CN.md`.
- `docs/ARCHITECTURE.md` — the driver package and the e2e environment, in the
  repository tree.
- `docs/images/hero-{light,dark}{,.zh-CN}.svg` — the `<desc>`, the driver count
  badge, and one drawn lane per family. Four files, and the art needs
  re-spacing rather than a text edit.
- `website/src/i18n/en.ts` and `zh.ts` — `meta.description`, `banner.text`,
  `hero.subtitle`, `drivers.supported`, `drivers.planned` and `roadmap.stages`.
  `planned` is the dangerous one: left alone it keeps asserting that a shipped
  driver is unbuilt.
- `frontend/src/i18n/locales/en.json` and `zh.json` —
  `page.settings.about.blurb` names the families.
- `frontend/src/App.tsx` and `frontend/src/mq/navigation.ts` — comments that
  count families.
- `.github/ISSUE_TEMPLATE/` — the broker dropdown in the bug and feature forms,
  in both languages, and the family's removal from the driver request form.
- `frontend/src/lib/alertRules.ts` and `frontend/src/lib/alertDerive.ts` — the
  rules a family can raise, and the dispatch that picks them. A driver added
  without both falls back to RocketMQ's rules, which read figures it does not
  report: an alerts page that looks armed and can never fire.
- `frontend/src/i18n/degradedReasons.test.ts` — the hand-copied list of every
  degraded reason **and caveat** a driver declares. Nothing ties a Go string to
  a JSON key, so this second copy is the only thing that goes red on a rename.
- `.github/labels.json` — a `driver:<family>` entry, then `npm run labels:sync`.
- `tests/e2e/<family>/compose.yaml`, the `e2e:<family>:*` scripts in
  `package.json`, and the shard in `.github/workflows/ci.yml`.
- `frontend/src/mq/navigation.<family>.test.ts`, and the Go test beside it that
  pins the same capability list. Two halves of one contract: a capability
  dropped in the driver takes a finished page out of the sidebar and nothing
  else notices, and a page added to the nav with no capability behind it is
  drawn and fails when opened. Neither half is worth much alone.
- `frontend/src/design/boards/<family>Boards.test.tsx` — every board through
  loading, failed, connected-but-empty and populated, against stubs shaped like
  what the driver actually sends. The i18n sweep renders each board once with
  nothing connected, which is the one state that does not touch the data.
- A family with no broker address declares **no** endpoint field at all, and
  `model.DriverDescriptor.RequiresEndpoints` reads that absence — nothing else
  decides it. Two things then need a second look:
  `frontend/src/design/data/connections.ts` prints the profile's endpoints in
  the connection row's address column, which would leave it blank, and the
  driver has to store its credentials under names of its own. `accessKey` and
  `secretKey` are reserved for RocketMQ's ACL: a family reusing them has them
  cleared on save and the global pair stamped on at dial time.
- A capability the family has and this API cannot answer belongs in
  `Degraded` with a reason, not left out and not filled in. The reason is an
  i18n key rather than a sentence, and it is copied by hand into
  `frontend/src/i18n/degradedReasons.test.ts` — nothing ties the Go string to
  the JSON key. Pub/Sub's backlog is the example: the number exists, in Cloud
  Monitoring, and the only way to produce one from the Pub/Sub API would be to
  pull the backlog and count it, which would deliver every message counted.

- The dead-letter page is answered three ways now, and which one a family
  gets is a real decision rather than a style. `CapDLQ` is a per-entity store
  the broker names and fills — a RocketMQ `%DLQ%` topic per consumer group, a
  Service Bus `$DeadLetterQueue` on every queue and subscription.
  `CapDeadLetterTopology` is an ordinary object something else points at, found
  by walking every object's configuration backwards — a RabbitMQ dead-letter
  exchange, an SQS redrive policy, a Pub/Sub dead-letter topic. Ask which of
  the two a family has before copying either: the shapes are not
  interchangeable, and the page reads very differently.

- A capability the shared vocabulary has no entry for is a new port, not a
  corner of `Attributes`. The map is documented as a contract between one
  driver's Go side and its own frontend module, so what goes in it is a
  variation on a canonical object - a permission, a durability flag, a
  replication factor. A Kinesis shard is not a variation on a partition: it has
  an identity, a hash key range and a parent, and putting that in a string map
  would have made it unreadable by anything except one board. Redis Stream set
  the precedent with four ports; ask which of the two a concept is before
  reaching for the map.

- A driver that needs a client library nobody has installed is a driver
  nobody can build. Before reaching for a vendor's SDK, check what it links
  against: `github.com/ibm-messaging/mq-golang` is cgo over IBM's native MQ
  libraries, and adding it would have put the redistributable client on the
  critical path of every `go build` and every CI job in this repository. Three
  families are reached over the vendor's own HTTP management plane instead -
  ActiveMQ through Jolokia, IBM MQ through the mqweb server's REST APIs, and
  Solace through SEMP v2 - and every one of them needed the standard library
  and nothing else. A management plane that is also the data plane is worth
  looking for.

- A figure whose name reads like the one you want is worth measuring before it
  is believed. Solace reports `spooledMsgCount` on every queue, which reads
  exactly like a current depth and is a lifetime statistic: `clearStats` sets
  it to zero on a full queue, and a drained queue keeps its high-water mark.
  The depth is `meta.count` on the queue's message collection instead. The same
  object reports its spool usage in bytes beside a quota in megabytes. Neither
  is documented as a trap and both were found by putting a known number of
  messages on a queue and reading every field back - which is what a new
  driver's first hour should be spent on.

- A family whose management plane authorises in more than one place needs the
  credential asked for more than once. IBM MQ's mqweb server maps its
  administrative and messaging REST interfaces to two roles, and a deployment
  may hold them on two accounts - so the form collects an optional second pair
  and the connection probes the second interface when it opens. The tier that
  did not answer is degraded with a reason, the way ActiveMQ's AMQP acceptor
  and MQTT's $SYS tree are. What must not happen is one credential silently
  standing in for the other: half a second pair is refused rather than
  completed from the first. Whether an empty second pair falls back to the
  first is the family's own answer rather than a convention - IBM MQ's does,
  because both interfaces read one user registry; Solace's does not, because a
  SEMP management user and a Message VPN's client-username are objects in
  different directories.

Do not count the families by eye. The capability declarations settle which
drivers answer which page:

```bash
grep -rn '^\s*model\.Cap<X>,\s*$' internal/driver/*/*.go
```

## Pull requests

Open it against `main`. The title is the commit subject — same Conventional
Commits format, since that is what a squash lands as. The template asks for
what, why, and how to verify; the third is the one reviewers actually need.

Keep a pull request to one thing. A driver, a fix, a refactor — not two of
them, and not a refactor smuggled in beside a fix.

Four checks report on a pull request:

- **Check** (`ci.yml`) — starts every broker environment and runs the full
  gate. This one is yours.
- **Build** (`website.yml`) — the marketing site, which renders this
  repository's own markdown at build time. A copy edit to a changelog can break
  it.
- **Package** — skipped on pull requests by design.
- **Workers Builds** — fails on every pull request branch and is **not** caused
  by your change. Preview deployments are not enabled on the account, so only
  the deployment from `main` can go green. Ignore it.

## Releases

Releases are cut by the maintainer; [RELEASE.md](RELEASE.md) documents the
process. Contributors do not need to bump a version — leave `package.json`
alone and put your entry under `Unreleased`.

## License

By contributing you agree that your work is licensed under the
[Apache License 2.0](LICENSE), the same as the rest of the project.
