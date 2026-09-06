#!/usr/bin/env bash
#
# Seeds the Google Pub/Sub E2E environment with a topology worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half of
# verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of the Pub/Sub API, and opening
# the app to see whether the boards say true things. Comparing zero against
# zero proves nothing.
#
# Everything it creates is named mqs-seed-*, so it never collides with a test's
# objects - those are mqs-test-* - or with anything made by hand. Pub/Sub names
# take letters, digits and - _ . ~ + %, must start with a letter and must be at
# least three characters, which is why the separator here is a hyphen.
#
# Safe to re-run: seeded subscriptions and topics are deleted first, so the
# counts below are what the project holds afterwards rather than what has
# accumulated.
#
# The shape it builds is chosen for the five things one empty topic cannot show:
#
#   - mqs-seed-orders fans out to two subscriptions, which is the one thing SQS
#     could not show at all: the same message delivered to two independent
#     readers, each with its own backlog.
#   - mqs-seed-orders-worker points its failures at a dead-letter topic, which
#     is the only topology Pub/Sub has.
#   - mqs-seed-dead-letters holds messages of its own and has a subscription on
#     it, so the dead-letter board has something to draw rather than an empty
#     row.
#   - mqs-seed-orphaned is a topic with no subscription at all. Every message
#     published to it is discarded on the spot, which is the fault this family
#     alerts on and the state a topic list must make visible.
#   - mqs-seed-quiet is an ordinary topic with one idle subscription, so a
#     board has a genuinely empty backlog to draw beside the busy ones.
#
# The body is Python because the work is a REST API and doing it in shell would
# mean parsing JSON with sed. It deliberately shares no code with the driver:
# the same independence the cross-check needs.
set -euo pipefail
exec python3 - "$@" <<'PY'
import base64
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

# The emulator, whose REST surface is the same one the real API serves. The
# host is overridable for a run against a real project, which is the only way
# to seed one - though a real project needs a credential this script has no
# way to hold, so that path is a bearer token in the environment.
HOST = os.environ.get("PUBSUB_EMULATOR_HOST", "127.0.0.1:8085")
PROJECT = os.environ.get("GOOGLE_CLOUD_PROJECT", "mq-studio-e2e")
TOKEN = os.environ.get("PUBSUB_BEARER_TOKEN", "")
BASE = f"http://{HOST}/v1/projects/{PROJECT}"

ORDERS = "mqs-seed-orders"
DEAD = "mqs-seed-dead-letters"
ORPHANED = "mqs-seed-orphaned"
QUIET = "mqs-seed-quiet"
TOPICS = [ORDERS, DEAD, ORPHANED, QUIET]

WORKER = "mqs-seed-orders-worker"
AUDIT = "mqs-seed-orders-audit"
DEAD_READER = "mqs-seed-dead-letters-reader"
IDLE = "mqs-seed-quiet-idle"
SUBSCRIPTIONS = [WORKER, AUDIT, DEAD_READER, IDLE]


def call(method, path, payload=None, quiet=False):
    url = f"{BASE}{path}"
    body = json.dumps(payload).encode() if payload is not None else None
    headers = {"content-type": "application/json"}
    if TOKEN:
        headers["authorization"] = f"Bearer {TOKEN}"
    request = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as err:
        if quiet:
            return None
        raise SystemExit(f"{method} {path} failed: {err.read().decode(errors='replace')}")
    except urllib.error.URLError as err:
        raise SystemExit(f"{method} {path} failed: {err}")


def alive():
    return call("GET", "/topics", quiet=True) is not None


if not alive():
    sys.exit(f"{BASE} is not answering; start it with: npm run e2e:google-pubsub:up")

print("==> removing anything left from a previous run")
# Subscriptions first: a topic can be deleted with subscriptions still on it,
# and what survives is a subscription pointing at _deleted-topic_ - which is a
# state the seed must not leave behind by accident, because a live test asserts
# it deliberately.
for name in SUBSCRIPTIONS:
    call("DELETE", f"/subscriptions/{name}", quiet=True)
for name in TOPICS:
    call("DELETE", f"/topics/{name}", quiet=True)


def topic(name):
    call("PUT", f"/topics/{name}", {})
    return f"projects/{PROJECT}/topics/{name}"


def subscription(name, on, **fields):
    payload = {"topic": on}
    payload.update(fields)
    call("PUT", f"/subscriptions/{name}", payload)


def publish(name, count, prefix, **attributes):
    messages = [
        {
            "data": base64.b64encode(f"{prefix}-{index}".encode()).decode(),
            "attributes": {"seedIndex": str(index), **attributes},
        }
        for index in range(1, count + 1)
    ]
    published = call("POST", f"/topics/{name}:publish", {"messages": messages})
    if len(published.get("messageIds") or []) != count:
        raise SystemExit(f"{name}: published {published} instead of {count} messages")


print(f"==> {DEAD}: the dead-letter topic, with a reader of its own")
dead_path = topic(DEAD)
subscription(DEAD_READER, dead_path, ackDeadlineSeconds=30)
publish(DEAD, 4, "gave-up", kind="deadLetter")

print(f"==> {ORDERS}: one topic, two subscriptions, 12 messages to each")
orders_path = topic(ORDERS)
subscription(
    WORKER,
    orders_path,
    ackDeadlineSeconds=20,
    deadLetterPolicy={"deadLetterTopic": dead_path, "maxDeliveryAttempts": 5},
    retryPolicy={"minimumBackoff": "10s", "maximumBackoff": "600s"},
)
subscription(AUDIT, orders_path, ackDeadlineSeconds=60, retainAckedMessages=True)
publish(ORDERS, 12, "order", kind="order")

print(f"==> {ORPHANED}: a topic with no subscription, so every publish is discarded")
topic(ORPHANED)
publish(ORPHANED, 3, "into-the-void")

print(f"==> {QUIET}: one idle subscription and nothing published")
quiet_path = topic(QUIET)
subscription(IDLE, quiet_path)

print("==> what the project holds now")
listed_topics = [t["name"] for t in (call("GET", "/topics?pageSize=1000").get("topics") or [])]
seeded_topics = sorted(name for name in listed_topics if "/mqs-seed-" in name)
if len(seeded_topics) != len(TOPICS):
    raise SystemExit(
        f"seeded {len(TOPICS)} topics and the project reports {len(seeded_topics)}: {seeded_topics}")

listed_subs = [
    s["name"] for s in (call("GET", "/subscriptions?pageSize=1000").get("subscriptions") or [])
]
seeded_subs = sorted(name for name in listed_subs if "/mqs-seed-" in name)
if len(seeded_subs) != len(SUBSCRIPTIONS):
    raise SystemExit(
        f"seeded {len(SUBSCRIPTIONS)} subscriptions and the project reports "
        f"{len(seeded_subs)}: {seeded_subs}")

for path in seeded_topics:
    name = path.rsplit("/", 1)[-1]
    attached = call("GET", f"/topics/{name}/subscriptions").get("subscriptions") or []
    print(f"    topic {name} · {len(attached)} subscription(s)")
for path in seeded_subs:
    name = path.rsplit("/", 1)[-1]
    detail = call("GET", f"/subscriptions/{name}")
    dead_letter = detail.get("deadLetterPolicy", {}).get("deadLetterTopic", "")
    print(
        f"    subscription {name} · on {detail['topic'].rsplit('/', 1)[-1]}"
        f" · ack {detail.get('ackDeadlineSeconds')}s"
        + (f" · dead letters to {dead_letter.rsplit('/', 1)[-1]}" if dead_letter else "")
    )

print("==> done")
PY
