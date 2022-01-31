resource "confluentcloud_service_account" "test_sa" {
  display_name = "test_sa"
  description = "description for test_sa"
}

resource "confluentcloud_environment" "test-env" {
  display_name = "Development"
}

resource "confluentcloud_kafka_cluster" "standard-cluster-on-aws" {
  display_name = "standard_kafka_cluster_on_aws"
  availability = "SINGLE_ZONE"
  cloud        = "AWS"
  region       = "us-west-2"
  standard {}

  environment {
    id = confluentcloud_environment.test-env.id
  }
}

resource "confluentcloud_role_binding" "cluster-example-rb" {
  principal = "User:${confluentcloud_service_account.test_sa.id}"
  role_name  = "CloudClusterAdmin"
  crn_pattern = confluentcloud_kafka_cluster.standard-cluster-on-aws.rbac_crn
}

resource "confluentcloud_kafka_topic" "orders" {
  kafka_cluster = confluentcloud_kafka_cluster.standard-cluster-on-aws.id
  topic_name = "orders"
  partitions_count = 4
  http_endpoint = confluentcloud_kafka_cluster.standard-cluster-on-aws.http_endpoint
  config = {
    "cleanup.policy" = "compact"
    "max.message.bytes" = "12345"
    "retention.ms" = "6789000"
  }
  credentials {
    key = var.kafka_api_key
    secret = var.kafka_api_secret
  }
}

resource "confluentcloud_role_binding" "topic-example-rb" {
  principal = "User:${confluentcloud_service_account.test_sa.id}"
  role_name  = "..."
  crn_pattern = "${confluentcloud_kafka_cluster.standard-cluster-on-aws.rbac_crn}/kafka=${confluentcloud_kafka_cluster.standard-cluster-on-aws.id}/topic=${confluentcloud_kafka_topic.orders.topic_name}"
}
