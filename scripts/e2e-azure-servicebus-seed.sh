#!/usr/bin/env bash
#
# Seeds the Azure Service Bus E2E environment.
#
# The work itself is in scripts/e2e-azure-servicebus-seed/, and it is Go rather
# than the shell-and-python every other seed here is written in because Service
# Bus takes a message over AMQP 1.0 and nothing else - its REST surface refuses
# a send outright - so a seed that publishes has to speak the protocol. That
# program's own comment describes the topology it builds and why.
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./scripts/e2e-azure-servicebus-seed "$@"
