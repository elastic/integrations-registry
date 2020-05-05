# Kafka integration

This integration collects logs and metrics from [https://kafka.apache.org](Kafka) servers.

## Compatibility

The `log` dataset module is tested with logs from Kafka 0.9, 1.1.0 and 2.0.0.

The `broker`, `consumer`, `consumergroup`, `partition` and `producer` metricsets are tested with Kafka 0.10.2.1, 1.1.0, 2.1.1, and 2.2.2.

<!-- TODO: Add a link to Jolokia "input" in Metricbeat -->
The `broker`, `consumer` and `producer` metricsets require Jolokia to fetch JMX metrics. Refer to the Metricbeat documentation about Jolokia for more information.

## Logs

### log

The `log` dataset collects and parses logs from Kafka servers.

The fields reported are:

{{fields "log"}}

## Metrics

### broker

<!-- TODO example event -->

The fields reported are:

{{fields "broker"}}

### consumer

<!-- TODO example event -->

The fields reported are:

{{fields "consumer"}}

### consumergroup

<!-- TODO example event -->

The fields reported are:

{{fields "consumergroup"}}

### partition

<!-- TODO example event -->

The fields reported are:

{{fields "partition"}}

### producer

<!-- TODO example event -->

The fields reported are:

{{fields "producer"}}