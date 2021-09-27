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

resource "confluentcloud_kafka_acl" "describe-basic-cluster" {
  kafka = confluentcloud_kafka.basic-cluster.http_endpoint.id
  resource_type = "CLUSTER"
  resource_name = "kafka-cluster"
  pattern_type = "LITERAL"
  principal = "User:12345"
  host = "*"
  operation = "DESCRIBE"
  permission = "ALLOW"
  http_endpoint = confluentcloud_kafka.basic-cluster.http_endpoint
  credentials {
    key = "<Kafka API Key for confluentcloud_kafka.basic-cluster>"
    secret = "<Kafka API Secret for confluentcloud_kafka.basic-cluster>"
  }
}
