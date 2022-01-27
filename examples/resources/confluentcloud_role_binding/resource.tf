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

resource "confluentcloud_role_binding" "example-rb" {
  principal = "User:${confluentcloud_service_account.test_sa.id}"
  role_name  = "CloudClusterAdmin"
  crn_pattern = confluentcloud_kafka_cluster.standard-cluster-on-aws.rbac_crn
}
