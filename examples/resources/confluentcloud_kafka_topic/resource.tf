resource "confluentcloud_environment" "test-env" {
  display_name = "Development"
}

resource "confluentcloud_kafka" "basic-cluster" {
  display_name = "basic_kafka_cluster"
  availability = "single_zone"
  cloud = "gcp"
  region = "us-central1"
  cluster_type = "basic"

  environment {
    id = confluentcloud_environment.test-env.id
  }
}

resource "confluentcloud_kafka_topic" "orders" {
  kafka = confluentcloud_kafka.basic-cluster.http_endpoint.id
  topic_name = "orders"
  partitions_count = 4
  replication_factor = 3
  http_endpoint = confluentcloud_kafka.basic-cluster.http_endpoint
  config = {
    "cleanup.policy" = "compact"
    "max.message.bytes" = "12345"
    "retention.ms" = "67890"
  }
  credentials {
    key = "<Kafka API Key for confluentcloud_kafka.basic-cluster>"
    secret = "<Kafka API Secret for confluentcloud_kafka.basic-cluster>"
  }
}
