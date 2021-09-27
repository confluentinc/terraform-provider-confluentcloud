provider "confluentcloud" {
  username = var.api_key    # optionally use CONFLUENT_CLOUD_API_KEY env var
  password = var.api_secret # optionally use CONFLUENT_CLOUD_API_SECRET env var

  # if you want to edit the waiting strategy of terraform apply you can optionally specify the status to wait for
  # wait_until    = "PROVISIONED"
}