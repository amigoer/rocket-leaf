#!/usr/bin/env bash
#
# Seeds the SQS E2E environment with queues worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half of
# verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of the AWS API, and opening the
# app to see whether the boards say true things. Comparing zero against zero
# proves nothing.
#
# Everything it creates is named MQS-SEED-*, so it never collides with a test's
# objects - those are MQS-TEST-* - or with anything made by hand. SQS queue
# names take letters, digits, hyphens and underscores only, which is why the
# separator here is a hyphen where every other family's seed uses a dot.
#
# Safe to re-run: seeded queues are deleted first, so the counts below are what
# the account holds afterwards rather than what has accumulated.
#
# The shape it builds is chosen for the four things one empty queue cannot show:
#
#   - MQS-SEED-orders holds a backlog and points its failures at a dead-letter
#     queue, which is the only topology SQS has.
#   - MQS-SEED-orders-dlq holds messages of its own, so the dead-letter board
#     has a depth to show rather than an empty row.
#   - MQS-SEED-delayed holds only delayed messages, which are counted
#     separately and are invisible to every other figure.
#   - MQS-SEED-orders.fifo is a FIFO queue, which behaves differently on send
#     and identically everywhere else - the assertion being that the boards do
#     not treat it as a second kind of object.
#
# The body is Python because the work is a signed AWS API and doing it in shell
# would mean either the AWS CLI as a dependency or signing SigV4 by hand.
set -euo pipefail
exec python3 - "$@" <<'PY'
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
ENDPOINT = os.environ.get("MQ_STUDIO_SQS_ENDPOINT", "http://127.0.0.1:4566")
REGION = os.environ.get("AWS_DEFAULT_REGION", "eu-west-1")
ACCESS_KEY = os.environ.get("AWS_ACCESS_KEY_ID", "test")
SECRET_KEY = os.environ.get("AWS_SECRET_ACCESS_KEY", "test")

ORDERS = "MQS-SEED-orders"
DLQ = "MQS-SEED-orders-dlq"
DELAYED = "MQS-SEED-delayed"
EMPTY = "MQS-SEED-empty"
FIFO = "MQS-SEED-orders.fifo"
QUEUES = [ORDERS, DLQ, DELAYED, EMPTY, FIFO]


def sign(payload, target):
    """SigV4 for one SQS JSON request.

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
        "content-type": "application/x-amz-json-1.0",
        "host": host,
        "x-amz-date": stamp,
        "x-amz-target": f"AmazonSQS.{target}",
    }
    signed_headers = ";".join(sorted(headers))
    canonical_headers = "".join(f"{k}:{headers[k]}\n" for k in sorted(headers))
    canonical = "\n".join(
        ["POST", "/", "", canonical_headers, signed_headers, body_hash])

    scope = f"{datestamp}/{REGION}/sqs/aws4_request"
    to_sign = "\n".join(
        ["AWS4-HMAC-SHA256", stamp, scope,
         hashlib.sha256(canonical.encode()).hexdigest()])

    def hmac_sha256(key, message):
        return hmac.new(key, message.encode(), hashlib.sha256).digest()

    key = hmac_sha256(f"AWS4{SECRET_KEY}".encode(), datestamp)
    key = hmac_sha256(key, REGION)
    key = hmac_sha256(key, "sqs")
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
            return json.loads(response.read()).get("services", {}).get("sqs") in (
                "available", "running")
    except (urllib.error.URLError, ValueError):
        # A real account has no health endpoint. One signed call is the probe
        # there, and it is the same call every operation below makes.
        return call("ListQueues", {"MaxResults": 1}, quiet=True) is not None


if not alive():
    sys.exit(f"{ENDPOINT} is not answering for sqs; start it with: npm run e2e:sqs:up")

print("==> removing anything left from a previous run")
# One at a time and ignored: on a first run every one of these is a missing
# queue, which is exactly the state the seed wants.
for name in QUEUES:
    found = call("GetQueueUrl", {"QueueName": name}, quiet=True)
    if found:
        call("DeleteQueue", {"QueueUrl": found["QueueUrl"]}, quiet=True)

# A deleted queue's name is refused for 60 seconds afterwards, so a re-run has
# to wait for the name rather than fail on it.
def create(name, attributes=None):
    payload = {"QueueName": name}
    if attributes:
        payload["Attributes"] = attributes
    deadline = time.time() + 90
    while True:
        created = call("CreateQueue", payload, quiet=True)
        if created:
            return created["QueueUrl"]
        if time.time() > deadline:
            raise SystemExit(f"could not create {name} within 90s; a delete is still settling")
        time.sleep(3)


def send(url, count, prefix, delay=None, group=None):
    for index in range(1, count + 1):
        payload = {"QueueUrl": url, "MessageBody": f"{prefix}-{index}"}
        if delay is not None:
            payload["DelaySeconds"] = delay
        if group is not None:
            payload["MessageGroupId"] = group
            payload["MessageDeduplicationId"] = f"{prefix}-{index}"
        call("SendMessage", payload)


print(f"==> {DLQ}: the dead-letter queue, holding 4 messages of its own")
dlq_url = create(DLQ)
dlq_arn = call("GetQueueAttributes", {
    "QueueUrl": dlq_url, "AttributeNames": ["QueueArn"]})["Attributes"]["QueueArn"]
send(dlq_url, 4, "dead")

print(f"==> {ORDERS}: 12 messages, redriving into {DLQ} after 3 receives")
orders_url = create(ORDERS, {
    "VisibilityTimeout": "30",
    "MessageRetentionPeriod": "345600",
    "RedrivePolicy": json.dumps({"deadLetterTargetArn": dlq_arn, "maxReceiveCount": "3"}),
})
send(orders_url, 12, "order")

print(f"==> {DELAYED}: 5 messages held back 15 minutes, so nothing is visible")
delayed_url = create(DELAYED)
send(delayed_url, 5, "later", delay=900)

print(f"==> {EMPTY}: nothing, so a board has a genuinely empty row to draw")
create(EMPTY)

print(f"==> {FIFO}: a FIFO queue with 6 messages in one group")
fifo_url = create(FIFO, {"FifoQueue": "true", "ContentBasedDeduplication": "false"})
send(fifo_url, 6, "fifo", group="seed")

print("==> what the region holds now")
listed = call("ListQueues", {"QueueNamePrefix": "MQS-SEED-", "MaxResults": 1000})
urls = listed.get("QueueUrls") or []
if len(urls) != len(QUEUES):
    raise SystemExit(
        f"seeded {len(QUEUES)} queues and the region reports {len(urls)}: {urls}")
for url in sorted(urls):
    attributes = call("GetQueueAttributes", {
        "QueueUrl": url, "AttributeNames": ["All"]})["Attributes"]
    print(f"    {url.rsplit('/', 1)[-1]}"
          f" visible={attributes['ApproximateNumberOfMessages']}"
          f" delayed={attributes['ApproximateNumberOfMessagesDelayed']}"
          f" inFlight={attributes['ApproximateNumberOfMessagesNotVisible']}"
          f"{' redrive=' + json.loads(attributes['RedrivePolicy'])['deadLetterTargetArn'].rsplit(':', 1)[-1] if 'RedrivePolicy' in attributes else ''}")

print("==> done")
PY
