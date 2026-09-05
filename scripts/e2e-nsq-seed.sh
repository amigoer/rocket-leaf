#!/usr/bin/env bash
#
# Seeds the NSQ E2E cluster with topics and channels worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half
# of verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of nsqd's HTTP API, and opening
# the app to see whether the boards say true things. Comparing zero against
# zero proves nothing.
#
# Everything it creates is named MQS.SEED.*, so it never collides with a
# test's objects - those are MQS.TEST.* - or with anything made by hand.
#
# Safe to re-run: seeded topics are deleted first, so the counts below are
# what the cluster holds afterwards rather than what has accumulated.
#
# The shape it builds is chosen for the three things a single-node, one-topic
# environment cannot show:
#
#   - MQS.SEED.orders exists on both nsqd, with a different depth on each. A
#     driver that reads one daemon and calls it the cluster gets this wrong,
#     and nothing else here would catch it.
#   - MQS.SEED.audit has a paused channel holding a backlog, which is what
#     "consumers are connected and nothing is moving" actually looks like.
#   - MQS.SEED.paused is a paused topic. Its messages stay in the topic's own
#     queue instead of being copied into its channel, which is the only state
#     in NSQ where a topic depth is not zero.
#
# The body is Python because the work is HTTP with query strings, and doing it
# in shell means a curl per message.
set -euo pipefail
exec python3 - "$@" <<'PY'
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

NSQD1 = os.environ.get("MQ_STUDIO_NSQD1_URL", "http://127.0.0.1:4151")
NSQD2 = os.environ.get("MQ_STUDIO_NSQD2_URL", "http://127.0.0.1:4153")

ORDERS = "MQS.SEED.orders"
AUDIT = "MQS.SEED.audit"
EVENTS = "MQS.SEED.events"
PAUSED = "MQS.SEED.paused"
TOPICS = [ORDERS, AUDIT, EVENTS, PAUSED]


def call(base, path, params=None, body=None, method="POST", quiet=False):
    url = base + path
    if params:
        url += "?" + urllib.parse.urlencode(params)
    request = urllib.request.Request(url, data=body, method=method)
    # Pins the modern response envelope. Without it nsqd answers in the
    # pre-1.0 status_code / status_txt / data shape.
    request.add_header("Accept", "application/vnd.nsq; version=1.0")
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            return response.read()
    except urllib.error.URLError as err:
        if quiet:
            return None
        raise SystemExit(f"{url} failed: {err}")


def alive(base):
    try:
        request = urllib.request.Request(base + "/ping")
        with urllib.request.urlopen(request, timeout=5):
            return True
    except urllib.error.URLError:
        return False


for daemon in (NSQD1, NSQD2):
    if not alive(daemon):
        sys.exit(f"{daemon} is not answering; start the cluster with: npm run e2e:nsq:up")

print("==> removing anything left from a previous run")
# One at a time and ignored: on a first run every one of these is a 404, and
# a topic that is not there is exactly the state the seed wants.
for daemon in (NSQD1, NSQD2):
    for topic in TOPICS:
        call(daemon, "/topic/delete", {"topic": topic}, body=b"", quiet=True)


def create(daemon, topic, channels=()):
    call(daemon, "/topic/create", {"topic": topic}, body=b"")
    for channel in channels:
        call(daemon, "/channel/create", {"topic": topic, "channel": channel}, body=b"")


def publish(daemon, topic, count, prefix, defer_ms=None):
    """One /mpub per batch. NSQ separates messages with a newline, so nothing
    published here may contain one."""
    if count == 0:
        return
    if defer_ms is not None:
        for index in range(1, count + 1):
            call(daemon, "/pub", {"topic": topic, "defer": defer_ms},
                 body=f"{prefix}-deferred-{index}".encode())
        return
    payload = "\n".join(f"{prefix}-{index}" for index in range(1, count + 1))
    call(daemon, "/mpub", {"topic": topic}, body=payload.encode())


print(f"==> {ORDERS}: two channels, 120 messages on nsqd and 40 on nsqd2")
# The channels are created before anything is published, because a channel
# only receives what arrives after it exists. A channel added afterwards would
# report a depth of zero beside a topic that has taken 120 messages, which is
# NSQ behaving correctly and reads as a broken seed.
#
# Only analytics is asked for on nsqd2 and both appear there. That is nsqd
# creating a topic: it asks every nsqlookupd which channels are registered for
# the name and creates each one locally, so a channel added anywhere reaches
# every daemon that later carries the topic. Worth knowing before reading a
# channel list and concluding a driver invented a row.
create(NSQD1, ORDERS, ["analytics", "archive"])
create(NSQD2, ORDERS, ["analytics"])
publish(NSQD1, ORDERS, 120, "orders")
publish(NSQD2, ORDERS, 40, "orders")

print(f"==> {ORDERS}: 3 more deferred an hour out, so deferred_count is not zero")
publish(NSQD1, ORDERS, 3, "orders", defer_ms=3_600_000)

print(f"==> {AUDIT}: 12 messages on a paused channel")
create(NSQD1, AUDIT, ["analytics"])
publish(NSQD1, AUDIT, 12, "audit")
call(NSQD1, "/channel/pause", {"topic": AUDIT, "channel": "analytics"}, body=b"")

print(f"==> {EVENTS}: empty, for the consumer in the compose file to attach to")
create(NSQD1, EVENTS)

print(f"==> {PAUSED}: a paused topic holding 5 messages its channel has not seen")
create(NSQD1, PAUSED, ["analytics"])
call(NSQD1, "/topic/pause", {"topic": PAUSED}, body=b"")
publish(NSQD1, PAUSED, 5, "paused")

print("==> what the cluster holds now")
for daemon in (NSQD1, NSQD2):
    stats = json.loads(call(daemon, "/stats", {"format": "json"}, method="GET"))
    for topic in stats.get("topics") or []:
        if not topic["topic_name"].startswith("MQS.SEED."):
            continue
        channels = ", ".join(
            f"{channel['channel_name']}={channel['depth']}"
            f"{'(paused)' if channel['paused'] else ''}"
            for channel in topic["channels"]
        )
        print(f"    {daemon} {topic['topic_name']}"
              f" depth={topic['depth']}{' (paused)' if topic['paused'] else ''}"
              f" messages={topic['message_count']} channels[{channels}]")

print("==> done")
PY
