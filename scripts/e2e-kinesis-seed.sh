#!/usr/bin/env bash
#
# Seeds the Kinesis E2E environment with streams worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half of
# verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of the AWS API, and opening the
# app to see whether the boards say true things. Comparing zero against zero
# proves nothing.
#
# Everything it creates is named MQS-SEED-*, so it never collides with a test's
# objects - those are MQS-TEST-* - or with anything made by hand. A Kinesis
# stream name takes letters, digits, underscores, hyphens and periods.
#
# Safe to re-run: seeded streams are deleted first, so the counts below are what
# the region holds afterwards rather than what has accumulated.
#
# The shape it builds is chosen for the five things one empty stream cannot show:
#
#   - MQS-SEED-orders has three shards and records on each of them, so a browse
#     has to read every shard rather than the first one.
#   - MQS-SEED-split has been split once, which is the whole reason this family
#     needed a shard concept: it leaves a closed parent that still holds the
#     records written before the split, and two children that name it as their
#     parent. A shard count alone cannot express any of that.
#   - MQS-SEED-empty holds nothing, so a board has a genuinely empty row.
#   - MQS-SEED-ondemand is an on-demand stream, whose capacity is the service's
#     to choose - the assertion being that the boards do not treat it as a
#     second kind of object.
#   - Two registered consumers on MQS-SEED-orders, because enhanced fan-out is
#     the only reader a Kinesis stream knows about at all.
#
# The body is Python because the work is a signed AWS API and doing it in shell
# would mean either the AWS CLI as a dependency or signing SigV4 by hand.
set -euo pipefail
exec python3 - "$@" <<'PY'
import base64
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

# A local emulator, so these are the strings the request has to carry rather
# than credentials that authorise anything. Overridable for a run against a
# real account, which is the only way to seed one.
ENDPOINT = os.environ.get("MQ_STUDIO_KINESIS_ENDPOINT", "http://127.0.0.1:4567")
REGION = os.environ.get("AWS_DEFAULT_REGION", "eu-west-1")
ACCESS_KEY = os.environ.get("AWS_ACCESS_KEY_ID", "test")
SECRET_KEY = os.environ.get("AWS_SECRET_ACCESS_KEY", "test")

ORDERS = "MQS-SEED-orders"
SPLIT = "MQS-SEED-split"
EMPTY = "MQS-SEED-empty"
ONDEMAND = "MQS-SEED-ondemand"
STREAMS = [ORDERS, SPLIT, EMPTY, ONDEMAND]
CONSUMERS = ["MQS-SEED-analytics", "MQS-SEED-archiver"]

# The whole 128-bit key space, which is what a stream's shards divide between
# them. Splitting at the midpoint of a shard's range is the only split that
# needs no arithmetic on the answer.
KEY_SPACE = 2 ** 128 - 1


def sign(payload, target):
    """SigV4 for one Kinesis JSON request.

    Written out rather than taken from a library because the seed must run on a
    plain runner with no pip install, and this is the whole of what signing a
    request to one service needs.
    """
    import datetime
    import hashlib
    import hmac

    now = datetime.datetime.now(datetime.timezone.utc)
    stamp = now.strftime("%Y%m%dT%H%M%SZ")
    datestamp = now.strftime("%Y%m%d")
    host = urllib.parse.urlparse(ENDPOINT).netloc
    body = json.dumps(payload).encode()
    body_hash = hashlib.sha256(body).hexdigest()

    headers = {
        "content-type": "application/x-amz-json-1.1",
        "host": host,
        "x-amz-date": stamp,
        "x-amz-target": f"Kinesis_20131202.{target}",
    }
    signed_headers = ";".join(sorted(headers))
    canonical_headers = "".join(f"{k}:{headers[k]}\n" for k in sorted(headers))
    canonical = "\n".join(
        ["POST", "/", "", canonical_headers, signed_headers, body_hash])

    scope = f"{datestamp}/{REGION}/kinesis/aws4_request"
    to_sign = "\n".join(
        ["AWS4-HMAC-SHA256", stamp, scope,
         hashlib.sha256(canonical.encode()).hexdigest()])

    def hmac_sha256(key, message):
        return hmac.new(key, message.encode(), hashlib.sha256).digest()

    key = hmac_sha256(f"AWS4{SECRET_KEY}".encode(), datestamp)
    key = hmac_sha256(key, REGION)
    key = hmac_sha256(key, "kinesis")
    key = hmac_sha256(key, "aws4_request")
    signature = hmac.new(key, to_sign.encode(), hashlib.sha256).hexdigest()

    headers["authorization"] = (
        f"AWS4-HMAC-SHA256 Credential={ACCESS_KEY}/{scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}")
    return body, headers


def call(target, payload, quiet=False):
    body, headers = sign(payload, target)
    request = urllib.request.Request(ENDPOINT, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as err:
        if quiet:
            return None
        raise SystemExit(f"{target} failed: {err.read().decode(errors='replace')}")
    except urllib.error.URLError as err:
        raise SystemExit(f"{target} failed: {err}")


def alive():
    try:
        with urllib.request.urlopen(f"{ENDPOINT}/_localstack/health", timeout=5) as response:
            return json.loads(response.read()).get("services", {}).get("kinesis") in (
                "available", "running")
    except (urllib.error.URLError, ValueError):
        # A real account has no health endpoint. One signed call is the probe
        # there, and it is the same call every operation below makes.
        return call("ListStreams", {"Limit": 1}, quiet=True) is not None


def summary(name, quiet=False):
    answer = call("DescribeStreamSummary", {"StreamName": name}, quiet=quiet)
    return answer["StreamDescriptionSummary"] if answer else None


def wait_active(name, seconds=90):
    """A stream is CREATING or UPDATING until the service says otherwise.

    Every call below that names one refuses while it is not ACTIVE, so this is
    a precondition rather than politeness.
    """
    deadline = time.time() + seconds
    while time.time() < deadline:
        found = summary(name, quiet=True)
        if found and found["StreamStatus"] == "ACTIVE":
            return found
        time.sleep(1)
    raise SystemExit(f"{name} did not become ACTIVE within {seconds}s")


def wait_gone(name, seconds=90):
    deadline = time.time() + seconds
    while time.time() < deadline:
        if summary(name, quiet=True) is None:
            return
        time.sleep(1)
    raise SystemExit(f"{name} was still there {seconds}s after being deleted")


def create(name, shards=None, on_demand=False):
    payload = {"StreamName": name}
    if on_demand:
        payload["StreamModeDetails"] = {"StreamMode": "ON_DEMAND"}
    else:
        payload["ShardCount"] = shards
    call("CreateStream", payload)
    return wait_active(name)


def put(name, count, prefix):
    """One PutRecords batch, with a partition key per record.

    The key is what decides the shard: Kinesis hashes it into the key space and
    the shard whose range covers the hash takes the record. Ten distinct keys
    is what spreads the batch over three shards rather than piling it on one.
    """
    records = [
        {"Data": base64.b64encode(f"{prefix}-{index}".encode()).decode(),
         "PartitionKey": f"key-{index % 10}"}
        for index in range(1, count + 1)
    ]
    answer = call("PutRecords", {"StreamName": name, "Records": records})
    failed = answer.get("FailedRecordCount", 0)
    if failed:
        raise SystemExit(f"{name}: {failed} of {count} records were rejected: {answer}")
    return answer["Records"]


def shards(name):
    listed, token, found = None, None, []
    while True:
        payload = {"NextToken": token} if token else {"StreamName": name}
        listed = call("ListShards", payload)
        found.extend(listed.get("Shards", []))
        token = listed.get("NextToken")
        if not token:
            return found


if not alive():
    sys.exit(f"{ENDPOINT} is not answering for kinesis; start it with: npm run e2e:kinesis:up")

print("==> removing anything left from a previous run")
# One at a time and ignored: on a first run every one of these is a missing
# stream, which is exactly the state the seed wants.
for name in STREAMS:
    if summary(name, quiet=True):
        # EnforceConsumerDeletion, because a stream with a registered consumer
        # refuses the delete outright rather than cascading.
        call("DeleteStream", {"StreamName": name, "EnforceConsumerDeletion": True}, quiet=True)
        wait_gone(name)

print(f"==> {ORDERS}: 3 shards, 30 records spread across them, 48h retention")
create(ORDERS, shards=3)
put(ORDERS, 30, "order")
call("IncreaseStreamRetentionPeriod", {"StreamName": ORDERS, "RetentionPeriodHours": 48})
wait_active(ORDERS)
orders_arn = summary(ORDERS)["StreamARN"]

print(f"==> {ORDERS}: {len(CONSUMERS)} registered consumers, which is the only reader a stream knows")
for consumer in CONSUMERS:
    call("RegisterStreamConsumer", {"StreamARN": orders_arn, "ConsumerName": consumer})

print(f"==> {SPLIT}: one shard, 8 records, then split - the parent closes and keeps them")
create(SPLIT, shards=1)
put(SPLIT, 8, "before-split")
parent = shards(SPLIT)[0]
midpoint = (int(parent["HashKeyRange"]["StartingHashKey"])
            + int(parent["HashKeyRange"]["EndingHashKey"])) // 2 + 1
call("SplitShard", {
    "StreamName": SPLIT,
    "ShardToSplit": parent["ShardId"],
    "NewStartingHashKey": str(midpoint),
})
wait_active(SPLIT)
put(SPLIT, 6, "after-split")

print(f"==> {EMPTY}: one shard and nothing on it, so a board has an empty row to draw")
create(EMPTY, shards=1)

print(f"==> {ONDEMAND}: an on-demand stream, whose capacity is the service's to choose")
create(ONDEMAND, on_demand=True)
put(ONDEMAND, 4, "ondemand")

print("==> what the region holds now")
listed, token, names = [], None, []
while True:
    payload = {"NextToken": token} if token else {}
    answer = call("ListStreams", payload)
    names.extend(answer.get("StreamNames", []))
    token = answer.get("NextToken")
    if not (token and answer.get("HasMoreStreams")):
        break
seeded = sorted(name for name in names if name.startswith("MQS-SEED-"))
if seeded != sorted(STREAMS):
    raise SystemExit(
        f"seeded {sorted(STREAMS)} and the region reports {seeded}")
for name in seeded:
    described = summary(name)
    listing = shards(name)
    closed = sum(1 for shard in listing
                 if shard["SequenceNumberRange"].get("EndingSequenceNumber"))
    print(f"    {name}"
          f" mode={described['StreamModeDetails']['StreamMode']}"
          f" openShards={described['OpenShardCount']}"
          f" closedShards={closed}"
          f" retentionHours={described['RetentionPeriodHours']}"
          f" consumers={described.get('ConsumerCount', 0)}")

split_shards = shards(SPLIT)
if len(split_shards) != 3:
    raise SystemExit(f"{SPLIT} should hold one closed parent and two children, not {split_shards}")
print("==> done")
PY
