#!/bin/sh
set -eu

create_topic() {
    /opt/kafka/bin/kafka-topics.sh \
        --bootstrap-server kafka:19092 \
        --create \
        --if-not-exists \
        --topic "$1" \
        --partitions "$2" \
        --replication-factor 1 \
        --config "retention.ms=$3"
}

create_topic feedflow.notification.requests.v1 6 1209600000
create_topic feedflow.notification.retry.1m.v1 6 172800000
create_topic feedflow.notification.retry.10m.v1 6 604800000
create_topic feedflow.notification.retry.1h.v1 6 1209600000
create_topic feedflow.notification.dlq.v1 6 7776000000
