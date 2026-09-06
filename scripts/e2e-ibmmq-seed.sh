#!/usr/bin/env bash
#
# Seeds the IBM MQ E2E queue manager with objects worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half
# of verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of the REST API, and opening the
# app to see whether the boards say true things. Comparing zero against zero
# proves nothing, and a fresh queue manager has none of the three shapes that
# matter here: a queue with a depth, a channel that is not running, and a
# message on the dead-letter queue.
#
# Everything it creates is named MQS.SEED.*, so it never collides with a
# test's objects - those are MQS.TEST.* - or with the image's own DEV.* ones.
#
# Safe to re-run: seeded objects are deleted first, so the counts printed at
# the end are what the queue manager holds rather than what has accumulated.
#
# The body is Python because the work is JSON over HTTPS with a self-signed
# certificate, and because two of the steps need a different credential from
# the others: the queue manager's objects are administered as admin and its
# messages are written as app, which is the mqweb role split the driver's
# second credential exists for.
set -euo pipefail
exec python3 - "$@" <<'PY'
import base64, json, os, ssl, subprocess, sys, time, urllib.error, urllib.request

BASE = os.environ.get("MQ_STUDIO_IBMMQ_URL", "https://127.0.0.1:9443")
QMGR = os.environ.get("MQ_STUDIO_IBMMQ_QMGR", "QM1")
ADMIN = os.environ.get("MQ_STUDIO_IBMMQ_ADMIN", "admin:passw0rd")
APP = os.environ.get("MQ_STUDIO_IBMMQ_APP", "app:passw0rd")
COMPOSE = os.environ.get("MQ_STUDIO_IBMMQ_COMPOSE", "tests/e2e/ibmmq/compose.yaml")

QUEUES = ["MQS.SEED.ORDERS", "MQS.SEED.AUDIT", "MQS.SEED.BACKOUT", "MQS.SEED.SUBQ"]
TOPIC = "MQS.SEED.EVENTS"
TOPIC_STRING = "dev/seed/events"
SUBSCRIPTION = "MQS.SEED.SUB"
CHANNEL = "MQS.SEED.SDR"
XMITQ = "MQS.SEED.XMITQ"
DEAD_LETTER_QUEUE = "DEV.DEAD.LETTER.QUEUE"

ORDERS = 12
PUBLICATIONS = 4
DEAD_LETTERS = 2

# The mqweb server presents a certificate it generated for itself, so the seed
# skips verification for the same reason the driver offers a switch for it -
# and, like the driver, only because it was asked to rather than by default.
UNVERIFIED = ssl.create_default_context()
UNVERIFIED.check_hostname = False
UNVERIFIED.verify_mode = ssl.CERT_NONE


def call(path, auth, method="GET", body=None, content_type=None, headers=None):
    request = urllib.request.Request(BASE + path, data=body, method=method)
    request.add_header("Authorization", "Basic " + base64.b64encode(auth.encode()).decode())
    if method != "GET":
        # Any value will do. The mqweb server checks only that the header is
        # there, because a browser cannot add one to a cross-site form post.
        request.add_header("ibm-mq-rest-csrf-token", "mq-studio-seed")
    if content_type:
        request.add_header("Content-Type", content_type)
    for key, value in (headers or {}).items():
        request.add_header(key, value)
    try:
        with urllib.request.urlopen(request, timeout=60, context=UNVERIFIED) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as err:
        return err.code, err.read()
    except urllib.error.URLError as err:
        sys.exit(f"the mqweb server at {BASE} is not answering ({err}); "
                 "start it with: npm run e2e:ibmmq:up")


def admin_json(path):
    status, payload = call(f"/ibmmq/rest/v1/admin{path}", ADMIN)
    if status != 200:
        sys.exit(f"GET {path} failed with {status}: {payload.decode(errors='replace')}")
    return json.loads(payload)


def mqsc(request, tolerate=(), optional=False):
    """One MQSC call. tolerate lists reason codes that are expected.

    Deleting an object that is not there answers with a reason code rather than
    an HTTP error, which is exactly what a re-run of this script produces - so
    the caller says which of those are fine rather than the helper guessing.
    optional goes further and tolerates any failure, which suits only the
    cleanup commands whose whole purpose is to leave nothing behind.
    """
    label = (request.get("parameters") or {}).get("command") or (
        f"{request.get('command')} {request.get('qualifier')}({request.get('name', '')})")
    status, payload = call(
        f"/ibmmq/rest/v1/admin/action/qmgr/{QMGR}/mqsc", ADMIN,
        method="POST", body=json.dumps(request).encode(), content_type="application/json")
    decoded = json.loads(payload)
    if status != 200 and "commandResponse" not in decoded:
        if optional:
            return decoded
        sys.exit(f"mqsc {label} failed: {payload.decode(errors='replace')}")
    for result in decoded.get("commandResponse", []):
        if result.get("completionCode", 0) == 0 or optional:
            continue
        if result.get("reasonCode") in tolerate:
            continue
        lines = result.get("message", []) + result.get("text", [])
        sys.exit(f"mqsc {label} failed: {' '.join(lines)}")
    return decoded


def define(qualifier, name, **parameters):
    parameters.setdefault("replace", "yes")
    mqsc({"type": "runCommandJSON", "command": "define", "qualifier": qualifier,
          "name": name, "parameters": parameters})


# The reason codes for "there was nothing to delete", which is the ordinary
# state of a first run. They are not one code: 2085 is the generic unknown
# object, and a channel (4032) and a subscription (2428) each answer with
# their own.
ABSENT = (2085, 2428, 3008, 4032)


def delete(qualifier, name, **parameters):
    mqsc({"type": "runCommandJSON", "command": "delete", "qualifier": qualifier,
          "name": name, "parameters": parameters or None}, tolerate=ABSENT)


def run_command(text, optional=False):
    """One MQSC command as text, for the verbs runCommandJSON has no shape for.

    START, STOP, REFRESH and SET AUTHREC are commands rather than object
    definitions, and the JSON form covers define, alter, delete and display.
    """
    return mqsc({"type": "runCommand", "parameters": {"command": text}}, optional=optional)


def put(queue, body):
    status, payload = call(
        f"/ibmmq/rest/v1/messaging/qmgr/{QMGR}/queue/{queue}/message", APP,
        method="POST", body=body.encode(), content_type="text/plain;charset=utf-8")
    if status != 201:
        sys.exit(f"putting to {queue} failed with {status}: {payload.decode(errors='replace')}")


def depth(queue):
    listing = admin_json(f"/qmgr/{QMGR}/queue/{queue}?status=*")
    entries = listing.get("queue", [])
    if not entries:
        sys.exit(f"{queue} does not exist after seeding")
    return entries[0].get("status", {}).get("currentDepth", -1)


def publish(count, prefix):
    """Publishes to the topic string from inside the container.

    The messaging REST interface has no topic resource - it carries messages to
    and from queues and nothing else - so a real publication has to come from a
    real MQ client, and the image ships one. This is also why the driver
    declares that a send goes to a queue.
    """
    payload = "".join(f"{prefix}-{index}\n" for index in range(count))
    result = subprocess.run(
        ["docker", "compose", "-f", COMPOSE, "exec", "-T", "ibmmq",
         "/opt/mqm/samp/bin/amqspub", TOPIC_STRING, QMGR],
        input=payload, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        sys.exit(f"publishing to {TOPIC_STRING} failed: {result.stderr.strip()}")


state = admin_json("/qmgr")
running = [entry["name"] for entry in state.get("qmgr", []) if entry.get("state") == "running"]
if QMGR not in running:
    sys.exit(f"{QMGR} is not running at {BASE}; start it with: npm run e2e:ibmmq:up")

print(f"seeding {QMGR} at {BASE}")

def channel_defined():
    response = mqsc({"type": "runCommandJSON", "command": "display", "qualifier": "channel",
                     "name": CHANNEL, "responseParameters": ["chltype"]}, tolerate=ABSENT)
    return any(entry.get("completionCode", 0) == 0 and entry.get("parameters")
               for entry in response.get("commandResponse", []))


# Remove what a previous run left, youngest dependency first: a subscription
# holds its destination queue open, and a channel holds its transmission queue.
#
# The channel is the one that has to be waited for. Stopping is asynchronous
# and a delete that arrives while the channel is still stopping is refused with
# AMQ8148E, so the delete is retried until the definition is gone rather than
# issued once and hoped for. FORCE rather than QUIESCE: the channel is retrying
# against an address that does not answer, and a quiesce would wait for a batch
# that will never start.
run_command(f"STOP CHANNEL({CHANNEL}) MODE(FORCE)", optional=True)
for _ in range(30):
    mqsc({"type": "runCommandJSON", "command": "delete", "qualifier": "channel",
          "name": CHANNEL}, optional=True)
    if not channel_defined():
        break
    time.sleep(1)
else:
    sys.exit(f"{CHANNEL} could not be deleted in 30s; it is still in use")
delete("sub", SUBSCRIPTION)
delete("topic", TOPIC)
for queue in QUEUES + [XMITQ]:
    delete("qlocal", queue, purge="yes")
# The dead-letter queue is the image's rather than the seed's, so it is emptied
# instead of deleted: without this the count below grows on every run and the
# cross-check would have no figure it could predict.
run_command(f"CLEAR QLOCAL({DEAD_LETTER_QUEUE})")

for queue in QUEUES:
    define("qlocal", queue)
print(f"  queues: {', '.join(QUEUES)}")

# The image grants its app account authority on DEV.** only, and nothing here
# is under it. The grant covers MQS.** rather than MQS.SEED.** because the live
# tests create MQS.TEST.* queues of their own and put messages on them through
# the same interface. It is the same shape the image's own dev config uses, and
# it is what lets the messaging interface browse and put here at all.
run_command("SET AUTHREC PROFILE('MQS.**') PRINCIPAL('app') "
            "OBJTYPE(QUEUE) AUTHADD(BROWSE,GET,INQ,PUT,DSP)")
run_command("REFRESH SECURITY(*) TYPE(AUTHSERV)")

# A backout queue and the queue that names it. Nothing about MQS.SEED.BACKOUT
# marks it as a dead-letter queue - it is an ordinary queue, and it is one only
# because MQS.SEED.AUDIT points at it, which is the whole reason this family
# answers the dead-letter page by walking the topology.
run_command("ALTER QLOCAL(MQS.SEED.AUDIT) BOQNAME(MQS.SEED.BACKOUT) BOTHRESH(3)")
print("  MQS.SEED.AUDIT backs out into MQS.SEED.BACKOUT after 3 attempts")

for index in range(ORDERS):
    put("MQS.SEED.ORDERS", json.dumps({"order": index, "seed": "mq-studio"}))
print(f"  MQS.SEED.ORDERS holds {depth('MQS.SEED.ORDERS')} messages")

define("topic", TOPIC, topicstr=TOPIC_STRING, descr="mq-studio seed topic")
define("sub", SUBSCRIPTION, topicstr=TOPIC_STRING, dest="MQS.SEED.SUBQ")
publish(PUBLICATIONS, "seed")
print(f"  {SUBSCRIPTION} on {TOPIC_STRING} is holding {depth('MQS.SEED.SUBQ')} publications")

# Dead letters, made the way a queue manager makes them: a publication it
# cannot put anywhere goes to the queue manager's DEADQ. Disabling put on the
# subscription's destination is what makes it undeliverable, and the messages
# that land carry a dead-letter header - which is why they are listed on the
# dead-letter board and their bodies are not readable through the REST API.
before = depth(DEAD_LETTER_QUEUE)
run_command("ALTER QLOCAL(MQS.SEED.SUBQ) PUT(DISABLED)")
publish(DEAD_LETTERS, "undeliverable")
run_command("ALTER QLOCAL(MQS.SEED.SUBQ) PUT(ENABLED)")
after = depth(DEAD_LETTER_QUEUE)
if after <= before:
    sys.exit(f"{DEAD_LETTER_QUEUE} still holds {after} messages; "
             "the seed produced no dead letter and the dead-letter board would be empty")
print(f"  {DEAD_LETTER_QUEUE} holds {after} messages")

# A channel that is not running, which a fresh queue manager cannot show: every
# channel it ships is defined and inactive, and the interesting row on the
# channels board is one that has been started and cannot connect. 192.0.2.10 is
# in TEST-NET-1, reserved for documentation, so it is unroutable everywhere.
define("qlocal", XMITQ, usage="xmitq")
define("channel", CHANNEL, chltype="sdr", conname="192.0.2.10(1414)", xmitq=XMITQ,
       descr="mq-studio seed channel that cannot connect")
run_command(f"START CHANNEL({CHANNEL})")
print(f"  {CHANNEL} started against an unroutable address")

channels = mqsc({"type": "runCommandJSON", "command": "display", "qualifier": "chstatus",
                 "name": CHANNEL, "responseParameters": ["status", "conname"]})
statuses = [entry.get("parameters", {}).get("status")
            for entry in channels.get("commandResponse", [])]
if not any(statuses):
    sys.exit(f"{CHANNEL} reports no status; the channels board would have nothing to show")
print(f"  {CHANNEL} status: {', '.join(status for status in statuses if status)}")

print("seed complete")
PY
