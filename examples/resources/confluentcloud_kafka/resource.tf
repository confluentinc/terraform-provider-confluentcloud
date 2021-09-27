resource "confluentcloud_environment" "test-env" {
  display_name = "Development"
}

resource "confluentcloud_kafka" "basic-cluster" {
  display_name = "basic_kafka_cluster"
  availability = "single_zone"
  cloud        = "aws"
  region       = "us-east-2"
  cluster_type = "basic"

  environment {
    id = confluentcloud_environment.test-env.id
  }
}
