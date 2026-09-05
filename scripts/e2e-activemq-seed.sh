#!/usr/bin/env bash
#
# Seeds both ActiveMQ E2E brokers with destinations worth looking at.
#
# The live tests do not need this: each one creates what it needs and removes
# it again, which is what keeps them independent. This is for the other half
# of verification - the cross-check, which compares figures the app computes
# against the same figures read straight out of Jolokia, and opening the app
# to see whether the boards say true things. Comparing zero against zero
# proves nothing, and an empty broker cannot show a queue depth, a dead
# letter, or a topic with subscriptions behind it.
#
# Everything it creates is named MQS.SEED.*, so it never collides with a
# test's objects - those are MQS.TEST.* - or with anything made by hand.
#
# Safe to re-run: seeded destinations are removed first, so the counts below
# are what the brokers hold afterwards rather than what has accumulated.
#
# The body is Python because the work is JSON: Jolokia takes a POSTed array of
# requests and answers with an array of results, and building those in shell
# meant a python3 -c per message and one HTTP round trip per message with it.
set -euo pipefail
exec python3 - "$@" <<'PY'
import base64, json, os, sys, urllib.error, urllib.request

ARTEMIS = (os.environ.get("MQ_STUDIO_ARTEMIS_URL", "http://127.0.0.1:8161/console/jolokia"),
           os.environ.get("MQ_STUDIO_ARTEMIS_AUTH", "artemis:artemis"))
CLASSIC = (os.environ.get("MQ_STUDIO_CLASSIC_URL", "http://127.0.0.1:8162/api/jolokia"),
           os.environ.get("MQ_STUDIO_CLASSIC_AUTH", "admin:admin"))

ARTEMIS_BROKER = 'org.apache.activemq.artemis:broker="0.0.0.0"'
CLASSIC_BROKER = "org.apache.activemq:type=Broker,brokerName=localhost"

QUEUES = ["MQS.SEED.orders", "MQS.SEED.audit", "MQS.SEED.dead"]
TOPIC = "MQS.SEED.events"
SUBSCRIPTIONS = ["analytics", "archive"]


def post(target, payload, quiet=False):
    """One Jolokia call. payload may be a single request or a list of them.

    Every request carries an Origin header. Both brokers ship
    jolokia-access.xml with <strict-checking/>, which rejects a request with
    no Origin as coming from the null origin - a 403 that reads like an
    authentication failure and is not one. The driver sends it for the same
    reason.
    """
    url, auth = target
    req = urllib.request.Request(url + "/", data=json.dumps(payload).encode(), method="POST")
    req.add_header("Origin", "http://localhost")
    req.add_header("Content-Type", "application/json")
    req.add_header("Authorization", "Basic " + base64.b64encode(auth.encode()).decode())
    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            return json.loads(response.read().decode())
    except urllib.error.URLError as err:
        if quiet:
            return None
        raise SystemExit(f"jolokia call to {url} failed: {err}")


def execute(mbean, operation, arguments):
    return {"type": "exec", "mbean": mbean, "operation": operation, "arguments": arguments}


def artemis_queue(address, name, routing):
    return (f'org.apache.activemq.artemis:address="{address}",broker="0.0.0.0"'
            f',component=addresses,queue="{name}",routing-type="{routing}"'
            ",subcomponent=queues")


def classic_queue(name):
    return ("org.apache.activemq:type=Broker,brokerName=localhost"
            f",destinationType=Queue,destinationName={name}")


def alive(target, pattern):
    url, auth = target
    req = urllib.request.Request(f"{url}/search/{pattern}")
    req.add_header("Origin", "http://localhost")
    req.add_header("Authorization", "Basic " + base64.b64encode(auth.encode()).decode())
    try:
        with urllib.request.urlopen(req, timeout=15):
            return True
    except urllib.error.URLError:
        return False


if not alive(ARTEMIS, "org.apache.activemq.artemis:broker=*"):
    sys.exit("artemis is not answering; start it with: npm run e2e:activemq:up")
if not alive(CLASSIC, "org.apache.activemq:type=Broker,brokerName=*"):
    sys.exit("classic is not answering; start it with: npm run e2e:activemq:classic:up")

print("==> removing anything left from a previous run")
# Sent one at a time and ignored: a bulk request whose members are all
# expected to fail on a first run tells us nothing, and Jolokia reports each
# member's failure in its own result rather than failing the batch.
for queue in QUEUES:
    post(ARTEMIS, execute(ARTEMIS_BROKER, "destroyQueue(java.lang.String,boolean,boolean)",
                          [queue, True, True]), quiet=True)
    post(CLASSIC, execute(CLASSIC_BROKER, "removeQueue(java.lang.String)", [queue]), quiet=True)
for name in SUBSCRIPTIONS:
    post(ARTEMIS, execute(ARTEMIS_BROKER, "destroyQueue(java.lang.String,boolean,boolean)",
                          [f"{TOPIC}.{name}", True, True]), quiet=True)
post(ARTEMIS, execute(ARTEMIS_BROKER, "deleteAddress(java.lang.String,boolean)", [TOPIC, True]), quiet=True)
post(CLASSIC, execute(CLASSIC_BROKER, "removeTopic(java.lang.String)", [TOPIC]), quiet=True)

print("==> artemis: three anycast queues, and a multicast address with two subscriptions")
post(ARTEMIS, [execute(ARTEMIS_BROKER, "createQueue(java.lang.String,java.lang.String,java.lang.String)",
                       [queue, queue, "ANYCAST"]) for queue in QUEUES])
# A multicast address with a queue under it per subscriber is what a durable
# subscription is on Artemis: the address is the topic, each queue is one
# subscriber's stored position. That is the shape the subscriptions board
# reads, and it is why Artemis needs no JMS client to seed one.
post(ARTEMIS, [execute(ARTEMIS_BROKER, "createQueue(java.lang.String,java.lang.String,java.lang.String)",
                       [TOPIC, f"{TOPIC}.{name}", "MULTICAST"]) for name in SUBSCRIPTIONS])

print("==> classic: three queues and a topic")
post(CLASSIC, [execute(CLASSIC_BROKER, "addQueue(java.lang.String)", [queue]) for queue in QUEUES])
post(CLASSIC, execute(CLASSIC_BROKER, "addTopic(java.lang.String)", [TOPIC]))
# Classic has no way to seed a durable subscriber: one exists because a JMS
# consumer registered it, and JMX offers no operation that creates one. The
# subscriptions board therefore reads an empty list here until a client
# connects, which is the truth about the broker rather than a gap in the seed.

# 500 on orders, deliberately over Classic's maxBrowsePageSize of 400. The
# browse cap is a caveat the driver reports rather than a bug, and a queue
# that never exceeds it would let a regression through unnoticed. Artemis
# pages properly and needs no such queue, so it gets a round 120.
DEPTHS = {"MQS.SEED.orders": (120, 500), "MQS.SEED.audit": (12, 12), "MQS.SEED.dead": (5, 5)}
BATCH = 50


def in_batches(target, requests):
    for start in range(0, len(requests), BATCH):
        post(target, requests[start:start + BATCH])


for queue, (artemis_depth, classic_depth) in DEPTHS.items():
    label = queue.rsplit(".", 1)[-1]
    print(f"==> {queue}: {artemis_depth} on artemis, {classic_depth} on classic")
    in_batches(ARTEMIS, [
        execute(artemis_queue(queue, queue, "anycast"),
                "sendMessage(java.util.Map,int,java.lang.String,boolean,java.lang.String,java.lang.String)",
                [{"seedIndex": str(i)}, 3, f"{label}-{i}", True, ARTEMIS[1].split(":")[0], ARTEMIS[1].split(":")[1]])
        for i in range(1, artemis_depth + 1)])
    in_batches(CLASSIC, [
        execute(classic_queue(queue), "sendTextMessage(java.lang.String)", [f"{label}-{i}"])
        for i in range(1, classic_depth + 1)])

print("==> done")
PY
