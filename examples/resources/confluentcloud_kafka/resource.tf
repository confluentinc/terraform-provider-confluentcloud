resource "confluentcloud_environment" "test-env" {
  display_name = "Development"
}

resource "confluentcloud_kafka_cluster" "basic-cluster" {
  display_name = "basic_kafka_cluster"
  availability = "SINGLE_ZONE"
  cloud        = "AWS"
  region       = "us-east-2"
  basic {}

  environment {
    id = confluentcloud_environment.test-env.id
  }
}
