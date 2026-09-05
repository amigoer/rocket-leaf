# MQ Studio Architecture

## Process model

```text
React UI (system WebView)
        │ Wails bindings — direct, in-process calls
internal/bridge      ── the only layer allowed to reshape data for the UI
        │
internal/service     ── domain logic
        │
RocketMQ Admin API / local encrypted settings
```

The application is a single process. The UI runs in the platform WebView
(WKWebView on macOS, WebView2 on Windows, WebKitGTK on Linux) and reaches Go
through generated bindings, so there is no local HTTP server, no auth token and
no child process to supervise.

## The bridge layer

`internal/bridge` exposes one Wails service per domain. Its methods delegate
straight to `internal/service`, and it is the only place that reshapes data on
the way out:

- Stored AccessKey / SecretKey values never leave the Go process. The bridge
  replaces them with `accessKeyConfigured` / `secretKeyConfigured` flags.
- Connection and settings updates carry an explicit credential mode:
  `preserve`, `replace` or `clear`.
- Config file paths and plaintext exports stay in Go: `SystemService` owns the
  file dialogs, reads and writes the file, and returns only the chosen path.
- External links are checked against a host allowlist before being handed to
  the OS browser.

`wails3 generate bindings` writes the TypeScript for these services into
`frontend/bindings/`, typed directly from the Go structs. `npm run check` fails
if the committed bindings drift from the Go source.

## Frontend seams

The UI never calls a binding directly. Two modules sit in between:

- `frontend/src/api/*.ts` wraps each bound service and is the only place that
  knows the binding shapes.
- `frontend/src/api/models.ts` re-exports the generated domain types under the
  vocabulary the pages use, including friendlier names for the generated enums.

Window chrome (minimise, maximise, close, drag regions) is driven by the Wails
window runtime from the frontend; only the native background colour, which has
to stay in step with the light/dark theme, goes through a Go service.

## macOS title bar

The renderer paints the title bar itself, so the native traffic lights have to
line up with it. Wails exposes no equivalent of Electron's
`trafficLightPosition`, so `internal/macwindow` moves the standard window
buttons through a small Objective-C shim. AppKit rebuilds the themed frame on
resize and when leaving fullscreen, which resets the buttons, so the shim keeps
a positioner attached to the window and re-applies the offset on those
notifications. It leaves the buttons alone while the window is fullscreen,
where AppKit owns them.

The geometry lives in two places that must agree: `titleBarHeight` and
`trafficLightLeft` in `main.go`, and the `.tb2` / `.tb2--mac` rules in
`frontend/src/design/tokens.css`, which keep the title bar clear of the
buttons instead of drawing stand-ins for them.

## Repository layout

```text
main.go                  Wails application entrypoint
internal/
  bridge/                Wails services exposed to the frontend
  app/                   Service wiring
  service/               Domain services, one package per domain
  driver/                The broker seam: ports, capabilities, conformance
    rocketmq/            RocketMQ driver
    rabbitmq/            RabbitMQ driver
    kafka/               Kafka driver
    pulsar/              Pulsar driver
    redisstream/         Redis Stream driver
    mqtt/                MQTT driver, with emqx/ for the vendor management API
    nats/                NATS driver, JetStream and the $SYS account
    activemq/            ActiveMQ driver: Classic and Artemis over Jolokia
  model/                 Domain models and the capability vocabulary
  crypto/                Local encryption helpers
  storage/               On-disk layout and atomic writes
  update/                In-app updater: check, download, verify, install
  macwindow/             Native macOS chrome Wails does not expose (cgo)
  tray/                  System tray
frontend/
  bindings/              Generated TypeScript bindings (committed)
  src/api/               Binding wrappers, domain types, platform access
  src/components/        shadcn/ui primitives and the app composites over them
  src/design/            The shell, the page registry, and every board
  src/hooks/             React hooks / providers
  src/mq/                Per-family attribute readers, navigation, capabilities
  src/lib/               Pure helpers: formatting, alert rules, storage
  src/i18n/              Locale bundles, zh and en
  src/styles/            Global CSS and early theme bootstrap
build/                   Wails build assets and per-platform Taskfiles
scripts/                 Version check, e2e seeds, packaging asset generators
tests/
  e2e/rocketmq/          RocketMQ e2e environment
  e2e/rocketmq-acl/      RocketMQ with ACL on, for the access-control tests
  e2e/rabbitmq/          RabbitMQ with the optional plugins on
  e2e/rabbitmq-plain/    RabbitMQ with none of them, for the degraded paths
  e2e/kafka/             Three-broker KRaft cluster
  e2e/kafka-secure/      Kafka with SASL and an authorizer
  e2e/pulsar/            Pulsar standalone with the admin API
  e2e/redis/             Redis standalone with ACL users
  e2e/redis-cluster/     Redis in cluster mode, for the multi-master paths
  e2e/mqtt/              Mosquitto: a $SYS tree and no management API
  e2e/mqtt-emqx/         EMQX: a management API and no readable $SYS
  e2e/nats/              Three-server NATS cluster with JetStream and $SYS
  e2e/nats-plain/        NATS with neither, for the degraded paths
  e2e/activemq/          ActiveMQ Artemis with its console and AMQP acceptor
  e2e/activemq-classic/  ActiveMQ Classic, the family's other product
  throughput-load/       Load generator for the throughput charts (own module)
```

## Build

`Taskfile.yml` drives everything through the `wails3` CLI: `wails3 task dev` for
hot reload, `wails3 task build` for a binary, `wails3 task package` for a
distributable. The app version lives in `package.json` and is injected into the
binary with `-ldflags "-X main.version=..."`.

Bumping it means editing `package.json`, both lockfiles, `frontend/package.json`
and `info.version` in `build/config.yml`, then running
`wails3 task common:update:build-assets` to regenerate the committed platform
manifests. Those manifests are what the packaged artifacts declare to the OS, so
`npm run check:version` verifies them too and names any that are stale.

The app id is `com.mqstudio.app` and the user data directory is `mq-studio`.
Both were renamed along with the app, and nothing carries pre-rename data
across: an install that predates the rename keeps its own directory untouched
and MQ Studio starts empty. Copying those files over by hand does not help
either, because `crypto.hkdfInfoPrefix` feeds key derivation and was renamed
too, so the stored `ENC:` values no longer decrypt.

`.github/workflows/ci.yml` runs the same gate on pushes and pull requests, but
splits it across jobs that run at the same time: the frontend build and tests
need neither Go nor Docker, the static checks are the only thing that needs the
GTK headers, and the live suites are sharded one job per broker family, each
starting only its own compose stacks.

That last split is not free. `MQ_STUDIO_E2E_FAMILIES` tells `internal/e2e`
which families a shard is responsible for, and the ones it does not name skip -
so a test can now go unrun without anything turning red, which is how issue #48
went unnoticed. Two things stop that. `TestEveryFamilyHasACIShard` pins
`e2e.AllFamilies` against the workflow's shard matrix, and the `coverage` job
runs `scripts/ci-coverage.mjs` over every shard's `go test -json` output to
assert that each test passed in at least one of them. A skip only counts as
deliberate if it did not come from the gate, which is what `e2e.SkipMarker`
distinguishes. `package` waits on that job, so an artifact is never produced
from a run that tested less than it should have.

`release.yml` packages a tag. Its runner matrix follows what each platform needs
to compile: macOS builds both slices from one SDK and joins them with `lipo`,
Windows cross-compiles both architectures from one runner because it does not
need cgo, and Linux does, so each architecture gets a runner of its own.
