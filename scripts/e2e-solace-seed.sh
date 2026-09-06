#!/usr/bin/env bash
#
# Seeds the Solace E2E broker with objects worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half of
# verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of SEMP, and opening the app to
# see whether the boards say true things. Comparing zero against zero proves
# nothing, and a fresh broker has none of the four shapes that matter here: a
# queue with a real backlog, a queue that dead-letters into another, a queue
# attracting messages through topic subscriptions, and a second Message VPN to
# switch the scope to.
#
# Everything it creates is named mqstudio/seed/*, so it never collides with a
# test's objects - those are mqstudio/test/* - or with the broker's own
# #-prefixed ones.
#
# Safe to re-run: seeded objects are deleted first, so the counts printed at
# the end are what the broker holds rather than what has accumulated.
#
# The body is Python because the work is JSON over HTTP on one port and message
# bodies over HTTP on another, and because the second Message VPN has to be
# created, configured and then published to on a port only the first one's
# configuration can tell us.
set -euo pipefail
exec python3 - "$@" <<'PY'
import base64, json, os, ssl, sys, time, urllib.error, urllib.parse, urllib.request

SEMP = os.environ.get("MQ_STUDIO_SOLACE_URL", "http://127.0.0.1:8080")
ADMIN = os.environ.get("MQ_STUDIO_SOLACE_ADMIN", "admin:admin")
VPN = os.environ.get("MQ_STUDIO_SOLACE_VPN", "default")
REST = os.environ.get("MQ_STUDIO_SOLACE_REST", "http://127.0.0.1:9000")

# The second Message VPN, which exists so the scope switcher has somewhere to
# switch to and so the cross-check can prove a board is reading one VPN rather
# than the broker. Its REST port is its own: the listen port is a per-VPN
# setting, and two VPNs cannot share one.
SECOND_VPN = "mqstudio-seed"
SECOND_REST_PORT = 9010
SECOND_REST = os.environ.get("MQ_STUDIO_SOLACE_REST2", "http://127.0.0.1:9010")

ORDERS = "mqstudio/seed/orders"
AUDIT = "mqstudio/seed/audit"
DMQ = "mqstudio/seed/dmq"
EVENTS = "mqstudio/seed/events"
ENDPOINT = "mqstudio/seed/endpoint"
SUBSCRIPTION = "mqstudio/seed/events/>"
SECOND_QUEUE = "mqstudio/seed/other"

ORDER_COUNT = 12
EVENT_COUNT = 5
DEAD_COUNT = 3
SECOND_COUNT = 2

# The broker presents a certificate it generated for itself if it has been
# given TLS, so a seed pointed at an https address skips verification for the
# same reason the driver offers a switch for it.
UNVERIFIED = ssl.create_default_context()
UNVERIFIED.check_hostname = False
UNVERIFIED.verify_mode = ssl.CERT_NONE


def call(base, path, method="GET", body=None, auth=None, content_type=None):
    request = urllib.request.Request(base + path, data=body, method=method)
    if auth:
        request.add_header("Authorization", "Basic " + base64.b64encode(auth.encode()).decode())
    if content_type:
        request.add_header("Content-Type", content_type)
    try:
        with urllib.request.urlopen(request, timeout=60, context=UNVERIFIED) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as err:
        return err.code, err.read()
    except urllib.error.URLError as err:
        sys.exit(f"{base} is not answering ({err}); start it with: npm run e2e:solace:up")


def semp(path, method="GET", payload=None, tolerate=()):
    """One SEMP call. tolerate lists error statuses that are expected.

    Deleting an object that is not there answers NOT_FOUND rather than an HTTP
    error, which is exactly what a first run produces - so the caller says
    which of those are fine rather than the helper guessing. The status is read
    out of the envelope rather than off the HTTP response: SEMP answers a
    missing object with HTTP 400 and puts NOT_FOUND inside.
    """
    body = json.dumps(payload).encode() if payload is not None else None
    status, raw = call(SEMP, "/SEMP/v2" + path, method=method, body=body,
                       auth=ADMIN, content_type="application/json" if body else None)
    decoded = json.loads(raw)
    error = decoded.get("meta", {}).get("error")
    if error:
        if error.get("status") in tolerate:
            return None
        sys.exit(f"{method} {path} failed: {error.get('status')} {error.get('description')}")
    if status >= 400:
        sys.exit(f"{method} {path} failed with {status}: {raw.decode(errors='replace')}")
    return decoded


def quote(name):
    return urllib.parse.quote(name, safe="")


def publish(base, destination, body, kind="QUEUE"):
    status, raw = call(base, f"/{kind}/{quote(destination)}", method="POST",
                       body=body.encode(), content_type="text/plain")
    if status != 200:
        sys.exit(f"publishing to {kind} {destination} failed with {status}: "
                 f"{raw.decode(errors='replace')}")


def depth(vpn, queue):
    """How many messages a queue is holding right now.

    Read from the message collection's own count rather than from the queue's
    spooledMsgCount, which is a statistic: it counts every message ever spooled
    and clearStats sets it to zero while the queue is still full. The seed
    prints depths, so using the wrong one here would print figures the boards
    disagree with.
    """
    answer = semp(f"/monitor/msgVpns/{quote(vpn)}/queues/{quote(queue)}/msgs?count=1")
    return answer["meta"]["count"]


def create_queue(vpn, name, **settings):
    payload = {
        "queueName": name,
        "accessType": "exclusive",
        "permission": "consume",
        "ingressEnabled": True,
        "egressEnabled": True,
    }
    payload.update(settings)
    semp(f"/config/msgVpns/{quote(vpn)}/queues", "POST", payload)


def delete_queue(vpn, name):
    semp(f"/config/msgVpns/{quote(vpn)}/queues/{quote(name)}", "DELETE",
         tolerate=("NOT_FOUND",))


state = semp(f"/monitor/msgVpns/{quote(VPN)}?select=state,enabled")["data"]
if state.get("state") != "up":
    sys.exit(f"{VPN} at {SEMP} is {state.get('state')}; start it with: npm run e2e:solace:up")

print(f"seeding {VPN} at {SEMP}")

# Remove what a previous run left. Order matters for the second Message VPN and
# only there: a VPN holding any endpoint refuses to be deleted and names one of
# them, so its queues go first and it is disabled before it is removed.
for queue in (ORDERS, AUDIT, DMQ, EVENTS):
    delete_queue(VPN, queue)
semp(f"/config/msgVpns/{quote(VPN)}/topicEndpoints/{quote(ENDPOINT)}", "DELETE",
     tolerate=("NOT_FOUND",))

existing = semp(f"/config/msgVpns/{quote(SECOND_VPN)}/queues?select=queueName",
                tolerate=("NOT_FOUND",))
for row in (existing or {}).get("data", []):
    delete_queue(SECOND_VPN, row["queueName"])
semp(f"/config/msgVpns/{quote(SECOND_VPN)}", "PATCH", {"enabled": False},
     tolerate=("NOT_FOUND",))
semp(f"/config/msgVpns/{quote(SECOND_VPN)}", "DELETE", tolerate=("NOT_FOUND",))

# A queue with a real backlog, which is what gives the boards a figure and the
# cross-check something other than zero to compare.
create_queue(VPN, ORDERS)
for index in range(ORDER_COUNT):
    publish(REST, ORDERS, json.dumps({"order": index, "seed": "mq-studio"}))
print(f"  {ORDERS} holds {depth(VPN, ORDERS)} messages")

# A dead message queue and the queue that names it. Nothing about
# mqstudio/seed/dmq marks it as one - it is an ordinary queue, and it is a dead
# message queue only because another queue points at it, which is the whole
# reason this family answers the dead-letter page by walking the topology.
#
# The dead letters are real rather than published straight onto the DMQ: the
# audit queue expires its messages after a second and hands them on, which is
# the path a broker actually takes. respectDmqEligibleEnabled is off so that
# every message qualifies whatever the publisher marked - the REST interface
# marks none of them eligible, so without this the messages would be discarded
# and the dead-letter board would be empty.
create_queue(VPN, DMQ)
create_queue(VPN, AUDIT, deadMsgQueue=DMQ, maxTtl=1, respectTtlEnabled=True,
             respectDmqEligibleEnabled=False, maxRedeliveryCount=2)
for index in range(DEAD_COUNT):
    publish(REST, AUDIT, json.dumps({"audit": index, "seed": "mq-studio"}))

deadline = time.time() + 30
while time.time() < deadline and depth(VPN, DMQ) < DEAD_COUNT:
    time.sleep(1)
dead = depth(VPN, DMQ)
if dead < DEAD_COUNT:
    sys.exit(f"{DMQ} holds {dead} of {DEAD_COUNT} messages after 30s; "
             "the seed produced no dead letter and the dead-letter board would be empty")
print(f"  {AUDIT} dead-letters into {DMQ}, which holds {dead} messages")

# A queue that receives through the topology rather than by name, and a topic
# endpoint whose routing is its own configuration. Together they are the two
# halves of the routing board.
create_queue(VPN, EVENTS)
semp(f"/config/msgVpns/{quote(VPN)}/queues/{quote(EVENTS)}/subscriptions", "POST",
     {"subscriptionTopic": SUBSCRIPTION})
for index in range(EVENT_COUNT):
    publish(REST, f"mqstudio/seed/events/{index}", f"event-{index}", kind="TOPIC")
routed = depth(VPN, EVENTS)
if routed < EVENT_COUNT:
    sys.exit(f"{EVENTS} holds {routed} of {EVENT_COUNT} messages; "
             f"the subscription {SUBSCRIPTION} attracted nothing and the routing board "
             "would show an edge that does not work")
print(f"  {EVENTS} subscribes to {SUBSCRIPTION} and holds {routed} messages")

semp(f"/config/msgVpns/{quote(VPN)}/topicEndpoints", "POST", {
    "topicEndpointName": ENDPOINT,
    "accessType": "exclusive",
    "permission": "consume",
    "ingressEnabled": True,
    "egressEnabled": True,
})
print(f"  {ENDPOINT} is a topic endpoint")

# A second Message VPN, so the scope switcher has somewhere to switch to and
# the cross-check can prove a board is reading one VPN rather than the broker.
# Its own REST port, because the listen port is a per-VPN setting.
semp("/config/msgVpns", "POST", {
    "msgVpnName": SECOND_VPN,
    "enabled": True,
    "authenticationBasicType": "none",
    "maxMsgSpoolUsage": 100,
    "maxConnectionCount": 10,
    "serviceRestIncomingPlainTextEnabled": True,
    "serviceRestIncomingPlainTextListenPort": SECOND_REST_PORT,
    "serviceRestMode": "messaging",
})
# A fresh Message VPN ships its default client-username shut down, and a send
# to it is refused with 403 until it is enabled.
semp(f"/config/msgVpns/{quote(SECOND_VPN)}/clientUsernames/default", "PATCH",
     {"enabled": True})
# Its default client-profile also forbids guaranteed messaging, which the one
# in the "default" VPN does not - the container enables it there on first boot
# and nowhere else. Without this a send to a queue in this VPN is refused with
# 503 "Service Unavailable" and an SMF flow error that says nothing about
# permissions, on a VPN that reports itself up with the queue plainly there.
semp(f"/config/msgVpns/{quote(SECOND_VPN)}/clientProfiles/default", "PATCH", {
    "allowGuaranteedMsgSendEnabled": True,
    "allowGuaranteedMsgReceiveEnabled": True,
    "allowGuaranteedEndpointCreateEnabled": True,
})
create_queue(SECOND_VPN, SECOND_QUEUE)
for index in range(SECOND_COUNT):
    publish(SECOND_REST, SECOND_QUEUE, f"other-{index}")
other = depth(SECOND_VPN, SECOND_QUEUE)
if other < SECOND_COUNT:
    sys.exit(f"{SECOND_QUEUE} in {SECOND_VPN} holds {other} of {SECOND_COUNT} messages; "
             "the second message vpn is not taking sends and the scope switcher would "
             "have nothing to show")
print(f"  {SECOND_VPN} serves rest on {SECOND_REST_PORT} and "
      f"{SECOND_QUEUE} holds {other} messages")

vpns = [row["msgVpnName"] for row in semp("/config/msgVpns?select=msgVpnName")["data"]]
print(f"  message vpns: {', '.join(sorted(vpns))}")
print("seed complete")
PY
