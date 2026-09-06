SHELL := /bin/sh
.DEFAULT_GOAL := help
.NOTPARALLEL:

# Optional cross-compilation target, for example: make build ARCH=amd64
ARCH ?=

.PHONY: help install install-ci bindings icons dev run build package dmg \
	test test-go test-frontend e2e e2e-up e2e-seed e2e-down \
	e2e-acl-up e2e-acl-down \
	e2e-rabbitmq-up e2e-rabbitmq-seed e2e-rabbitmq-down \
	e2e-rabbitmq-plain-up e2e-rabbitmq-plain-down \
	e2e-kafka-up e2e-kafka-seed e2e-kafka-down \
	e2e-kafka-secure-up e2e-kafka-secure-down \
	e2e-pulsar-up e2e-pulsar-seed e2e-pulsar-down \
	e2e-redis-up e2e-redis-seed e2e-redis-down \
	e2e-redis-cluster-up e2e-redis-cluster-down \
	e2e-mqtt-up e2e-mqtt-down e2e-mqtt-emqx-up e2e-mqtt-emqx-down \
	e2e-nats-up e2e-nats-seed e2e-nats-down \
	e2e-nats-plain-up e2e-nats-plain-down \
	e2e-activemq-up e2e-activemq-seed e2e-activemq-down \
	e2e-activemq-classic-up e2e-activemq-classic-down \
	e2e-nsq-up e2e-nsq-seed e2e-nsq-down \
	e2e-sqs-up e2e-sqs-seed e2e-sqs-down \
	e2e-google-pubsub-up e2e-google-pubsub-seed e2e-google-pubsub-down \
	e2e-azure-servicebus-up e2e-azure-servicebus-seed e2e-azure-servicebus-down \
	e2e-kinesis-up e2e-kinesis-seed e2e-kinesis-down \
	e2e-ibmmq-up e2e-ibmmq-seed e2e-ibmmq-down \
	e2e-solace-up e2e-solace-seed e2e-solace-down \
	check ci clean \
	website-dev website-build

help: ## Show all available targets
	@awk 'BEGIN { FS = ":.*## " } /^[a-zA-Z0-9_.-]+:.*## / { printf "  %-20s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

install: ## Install root and frontend dependencies
	npm install
	npm install --prefix frontend

install-ci: ## Install dependencies from lockfiles (CI)
	npm ci
	npm ci --prefix frontend

bindings: ## Regenerate the TypeScript bindings from the Go services
	npm run generate:bindings

icons: ## Regenerate platform icons from build/appicon.png
	wails3 task icons

dev: ## Run the app with frontend hot reload
	wails3 task dev

run: ## Build and run the app
	wails3 task run

build: ## Build the app for the current platform
	wails3 task build $(if $(ARCH),ARCH=$(ARCH),)

package: ## Package a distributable build for the current platform
	wails3 task package $(if $(ARCH),ARCH=$(ARCH),)

dmg: ## Build the macOS disk image (needs: pipx install dmgbuild)
	wails3 task darwin:package:dmg $(if $(ARCH),ARCH=$(ARCH),)

test: ## Run Go and frontend unit tests
	npm test

test-go: ## Run all Go tests
	npm run test:go

test-frontend: ## Run frontend unit tests
	npm run test:frontend

e2e-up: ## Start RocketMQ 5.3.2 with OrbStack or Docker
	npm run e2e:up

e2e-seed: ## Seed the E2E broker with the topic and consumer group the live tests need
	npm run e2e:seed

e2e-acl-up: ## Start the ACL-enabled RocketMQ used by the ACL live tests
	npm run e2e:acl:up

e2e-acl-down: ## Stop the ACL E2E environment and remove its volumes
	npm run e2e:acl:down

e2e-rabbitmq-up: ## Start RabbitMQ 4 with the shovel, federation and stream plugins on
	npm run e2e:rabbitmq:up

e2e-rabbitmq-seed: ## Seed the RabbitMQ E2E broker with a topology worth looking at
	npm run e2e:rabbitmq:seed

e2e-rabbitmq-down: ## Stop the RabbitMQ E2E environment and remove its volumes
	npm run e2e:rabbitmq:down

e2e-rabbitmq-plain-up: ## Start the plugin-free RabbitMQ used by the degraded-path tests
	npm run e2e:rabbitmq:plain:up

e2e-rabbitmq-plain-down: ## Stop the plugin-free RabbitMQ environment
	npm run e2e:rabbitmq:plain:down

e2e-kafka-up: ## Start the three-broker KRaft Kafka cluster the live tests use
	npm run e2e:kafka:up

e2e-kafka-seed: ## Seed the Kafka cluster with topics, records and consumer groups
	npm run e2e:kafka:seed

e2e-kafka-down: ## Stop the Kafka E2E cluster and remove its volumes
	npm run e2e:kafka:down

e2e-kafka-secure-up: ## Start the SASL and authorizer Kafka used by the access-control tests
	npm run e2e:kafka:secure:up

e2e-kafka-secure-down: ## Stop the secure Kafka environment
	npm run e2e:kafka:secure:down

e2e-pulsar-up: ## Start the Pulsar E2E environment
	npm run e2e:pulsar:up

e2e-pulsar-seed: ## Seed the Pulsar E2E environment for the cross-check
	npm run e2e:pulsar:seed

e2e-pulsar-down: ## Stop the Pulsar E2E environment
	npm run e2e:pulsar:down
e2e-redis-up: ## Start the ACL-enabled Redis the live tests use
	npm run e2e:redis:up

e2e-redis-seed: ## Seed Redis with streams, groups and a pending entries list
	npm run e2e:redis:seed

e2e-redis-down: ## Stop the Redis environment and remove its volumes
	npm run e2e:redis:down

e2e-redis-cluster-up: ## Start the six-node Redis cluster used by the cluster tests
	npm run e2e:redis:cluster:up

e2e-redis-cluster-down: ## Stop the Redis cluster environment
	npm run e2e:redis:cluster:down
e2e-mqtt-up: ## Start the Mosquitto used by the protocol and $SYS live tests
	npm run e2e:mqtt:up

e2e-mqtt-down: ## Stop the Mosquitto E2E environment
	npm run e2e:mqtt:down

e2e-mqtt-emqx-up: ## Start the EMQX used by the management-plane live tests
	npm run e2e:mqtt:emqx:up

e2e-mqtt-emqx-down: ## Stop the EMQX E2E environment
	npm run e2e:mqtt:emqx:down

e2e-nats-up: ## Start the three-server NATS cluster used by the JetStream live tests
	npm run e2e:nats:up

e2e-nats-seed: ## Fill the NATS cluster with streams and consumers for the cross-check
	npm run e2e:nats:seed

e2e-nats-down: ## Stop the NATS cluster environment
	npm run e2e:nats:down

e2e-nats-plain-up: ## Start the NATS server with JetStream and the system account off
	npm run e2e:nats:plain:up

e2e-nats-plain-down: ## Stop the JetStream-free NATS environment
	npm run e2e:nats:plain:down

e2e-activemq-up: ## Start the Artemis broker used by the ActiveMQ live tests
	npm run e2e:activemq:up

e2e-activemq-seed: ## Fill both ActiveMQ brokers with destinations for the cross-check
	npm run e2e:activemq:seed

e2e-activemq-down: ## Stop the Artemis environment
	npm run e2e:activemq:down

e2e-activemq-classic-up: ## Start the ActiveMQ Classic broker, the family's other product
	npm run e2e:activemq:classic:up

e2e-activemq-classic-down: ## Stop the ActiveMQ Classic environment
	npm run e2e:activemq:classic:down

e2e-nsq-up: ## Start the two-nsqd, two-nsqlookupd NSQ cluster the live tests use
	npm run e2e:nsq:up

e2e-nsq-seed: ## Fill the NSQ cluster with topics and channels for the cross-check
	npm run e2e:nsq:seed

e2e-nsq-down: ## Stop the NSQ environment and remove its volumes
	npm run e2e:nsq:down

e2e-sqs-up: ## Start the LocalStack SQS environment the live tests use
	npm run e2e:sqs:up

e2e-sqs-seed: ## Fill the SQS region with queues for the cross-check
	npm run e2e:sqs:seed

e2e-sqs-down: ## Stop the SQS environment and remove its volumes
	npm run e2e:sqs:down

e2e-google-pubsub-up: ## Start the Google Pub/Sub emulator the live tests use
	npm run e2e:google-pubsub:up

e2e-google-pubsub-seed: ## Fill the Pub/Sub project with topics and subscriptions for the cross-check
	npm run e2e:google-pubsub:seed

e2e-google-pubsub-down: ## Stop the Pub/Sub environment and remove its volumes
	npm run e2e:google-pubsub:down

e2e-azure-servicebus-up: ## Start the Azure Service Bus emulator the live tests use
	npm run e2e:azure-servicebus:up

e2e-azure-servicebus-seed: ## Fill the Service Bus namespace with entities for the cross-check
	npm run e2e:azure-servicebus:seed

e2e-azure-servicebus-down: ## Stop the Service Bus environment and remove its volumes
	npm run e2e:azure-servicebus:down

e2e-kinesis-up: ## Start the LocalStack Kinesis environment the live tests use
	npm run e2e:kinesis:up

e2e-kinesis-seed: ## Fill the Kinesis region with streams for the cross-check
	npm run e2e:kinesis:seed

e2e-kinesis-down: ## Stop the Kinesis environment and remove its volumes
	npm run e2e:kinesis:down

e2e-ibmmq-up: ## Start the IBM MQ environment the live tests use
	npm run e2e:ibmmq:up

e2e-ibmmq-seed: ## Fill the queue manager with objects for the cross-check
	npm run e2e:ibmmq:seed

e2e-ibmmq-down: ## Stop the IBM MQ environment and remove its volumes
	npm run e2e:ibmmq:down

e2e-solace-up: ## Start the Solace environment the live tests use
	npm run e2e:solace:up

e2e-solace-seed: ## Fill the broker with objects for the cross-check
	npm run e2e:solace:seed

e2e-solace-down: ## Stop the Solace environment and remove its volumes
	npm run e2e:solace:down

e2e: ## Run the live tests against a running, seeded RocketMQ E2E environment
	npm run test:e2e

e2e-down: ## Stop the RocketMQ E2E environment and remove test volumes
	npm run e2e:down

website-dev: ## Run the marketing site with hot reload
	npm run website:dev

website-build: ## Build the marketing site into website/out
	npm run website:build

check: ## Run version, frontend build, gofmt, vet, tests, and bindings drift checks
	npm run check

ci: install-ci check ## Run baseline CI checks without Docker

clean: ## Remove build artifacts
	rm -rf bin frontend/dist
