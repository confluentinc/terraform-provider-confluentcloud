---
page_title: "Confluent Cloud Provider 0.4.0: Upgrade Guide"
---
# Confluent Cloud Provider 0.4.0: Upgrade Guide

This guide is intended to help with the upgrading process and focuses only on the changes necessary to upgrade to version `0.4.0` from one of the earlier versions (`0.1.0`, `0.2.0`, and `0.3.0`). 

!> **Warning:** Don't forget to create a backup of `terraform.tfstate` state file before the upgrading. 

## Upgrade Notes

- [Provider Version Configuration](#provider-version-configuration)
- [Updating Terraform Configuration](#updating-terraform-configuration)
- [Upgrade State File](#upgrade-state-file)

## Provider Version Configuration

-> **Note:** This guide uses the Terraform configuration from [Sample Project](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/guides/sample-project) as an example of a Terraform configuration that has a cluster and 2 ACLs.

Before upgrading to version `0.4.0`, ensure that your environment
successfully runs [`terraform plan`](https://www.terraform.io/docs/commands/plan.html)
without unexpected changes. Run the following command:
```bash
terraform plan
```
Your output should resemble:
```
confluentcloud_service_account.test-sa: Refreshing state... [id=sa-xyz123]
confluentcloud_environment.test-env: Refreshing state... [id=env-dge456]
confluentcloud_kafka_cluster.test-basic-cluster: Refreshing state... [id=lkc-abc123]
confluentcloud_kafka_acl.describe-test-basic-cluster: Refreshing state... [id=lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:12345#*#DESCRIBE#ALLOW]
confluentcloud_kafka_topic.orders: Refreshing state... [id=lkc-abc123/orders]
confluentcloud_kafka_acl.describe-orders: Refreshing state... [id=lkc-n2kvd/TOPIC#orders#LITERAL#User:12345#*#DESCRIBE#ALLOW]
...
No changes. Infrastructure is up-to-date.
```

The next step is to set the latest version `0.4.0` in a `required_providers` block of your Terraform configuration and run `terraform init` to download the new version.

The existing configuration:
```hcl
terraform {
  required_providers {
   # ...
   confluentcloud = {
      source  = "confluentinc/confluentcloud"
      version = "0.3.0" # or 0.1.0, 0.2.0
    }
  }
}
```

The updated configuration:
```hcl
terraform {
  required_providers {
   # ...
   confluentcloud = {
      source  = "confluentinc/confluentcloud"
      version = "0.4.0"
    }
  }
}
```

## Updating Terraform Configuration

### `confluentcloud_kafka_acl` resource

`principal` attribute started accepting service account IDs instead of integer IDs:

#### Before
```hcl
resource "confluentcloud_kafka_acl" "describe-orders" {
  # ...
  principal = "User:12345"
  # ...
}
```

#### After
```hcl
resource "confluentcloud_kafka_acl" "describe-orders" {
  # ...
  principal = "User:${confluentcloud_service_account.test-sa.id}"
  # principal = "User:${data.confluentcloud_service_account.test-sa.id}"
  # principal = "User:sa-xyz123"
  # ...
}
```

Update your Terraform configuration to reflect the changes to `confluentcloud_kafka_acl` resource above.

-> **Note:** If you are upgrading from either `0.1.0` or `0.2.0` you should also [remove](https://www.terraform.io/docs/cli/commands/state/rm.html) existing `confluentcloud_kafka_acl` resource instances from your `terraform.tfstate` state file.

### `confluentcloud_kafka_cluster` resource
New `rbac_crn` attribute was introduced to `confluentcloud_kafka_cluster` resource.

## Upgrade State File
-> **Note:** We recommend you to use Terraform version manager [tfutils/tfenv](https://github.com/tfutils/tfenv): `tfenv install 0.14.0`, `tfenv use 0.14.0`.

After updating for value of `principal` attribute in your Terraform configuration on a previous step, please run `terraform refresh` command.

```bash
terraform refresh
```
Your output should resemble:
```
confluentcloud_environment.test-env: Refreshing state... [id=env-dge456]
confluentcloud_service_account.test-sa: Refreshing state... [id=sa-xyz123]
confluentcloud_kafka_cluster.test-basic-cluster: Refreshing state... [id=lkc-abc123]
confluentcloud_kafka_acl.describe-test-basic-cluster: Refreshing state... [id=lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:514609#*#DESCRIBE#ALLOW]
confluentcloud_kafka_topic.orders: Refreshing state... [id=lkc-abc123/orders]
confluentcloud_kafka_acl.describe-orders: Refreshing state... [id=lkc-abc123/TOPIC#orders#LITERAL#User:514609#*#DESCRIBE#ALLOW]
confluentcloud_kafka_acl.describe-orders: Refreshing state... [id=lkc-abc123/TOPIC#orders#LITERAL#User:514609#*#DESCRIBE#ALLOW]
```
Run `terraform apply` to recreate 2 ACLs:
```bash
terraform apply
```
Your output should resemble:
```
...
  # confluentcloud_kafka_acl.describe-orders will be created
  + resource "confluentcloud_kafka_acl" "describe-orders" {
...
  # confluentcloud_kafka_acl.describe-test-basic-cluster will be created
  + resource "confluentcloud_kafka_acl" "describe-test-basic-cluster" {
...
confluentcloud_kafka_acl.describe-orders: Creating...
confluentcloud_kafka_acl.describe-test-basic-cluster: Creation complete after 1s [id=lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW]
confluentcloud_kafka_acl.describe-orders: Creation complete after 1s [id=lkc-abc123/TOPIC#orders#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW]
Apply complete! Resources: 2 added, 0 changed, 0 destroyed.
```

##### Before the Upgrade
```
$ confluent kafka acl list --cluster lkc-abc123 --environment env-dge456
    Principal    | Permission | Operation | ResourceType | ResourceName  | PatternType
-----------------+------------+-----------+--------------+---------------+--------------
  User:sa-xyz123 | ALLOW      | DESCRIBE  | TOPIC        | orders        | LITERAL
  User:sa-xyz123 | ALLOW      | DESCRIBE  | CLUSTER      | kafka-cluster | LITERAL

$ cat terraform.tfstate | grep "User:"
            "id": "lkc-abc123/TOPIC#orders#LITERAL#User:514609#*#DESCRIBE#ALLOW",
            "principal": "User:514609",
            "id": "lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:514609#*#DESCRIBE#ALLOW",
            "principal": "User:514609",
$ cat terraform.tfstate | grep "rbac" # 0 matches
```
##### After the Upgrade
```
$ confluent kafka acl list --cluster lkc-abc123 --environment env-dge456
    Principal    | Permission | Operation | ResourceType | ResourceName  | PatternType
-----------------+------------+-----------+--------------+---------------+--------------
  User:sa-xyz123 | ALLOW      | DESCRIBE  | TOPIC        | orders        | LITERAL
  User:sa-xyz123 | ALLOW      | DESCRIBE  | CLUSTER      | kafka-cluster | LITERAL

$ cat terraform.tfstate | grep "User:"
            "id": "lkc-abc123/TOPIC#orders#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW",
            "principal": "User:sa-xyz123",
            "id": "lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW",
            "principal": "User:sa-xyz123",
$ cat terraform.tfstate | grep "rbac"
            "rbac_crn": "crn://confluent.cloud/organization=1111aaaa-11aa-11aa-11aa-111111aaaaaa/environment=env-dge456/cloud-cluster=lkc-abc123",
```

##### Sanity Check

Check that the upgrade was successful by ensuring that your environment
successfully runs [`terraform plan`](https://www.terraform.io/docs/commands/plan.html)
without unexpected changes. Run the following command:
```bash
terraform plan
```
Your output should resemble:
```
confluentcloud_service_account.test-sa: Refreshing state... [id=sa-xyz123]
confluentcloud_environment.test-env: Refreshing state... [id=env-dge456]
confluentcloud_kafka_cluster.test-basic-cluster: Refreshing state... [id=lkc-abc123]
confluentcloud_kafka_acl.describe-test-basic-cluster: Refreshing state... [id=lkc-abc123/CLUSTER#kafka-cluster#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW]
confluentcloud_kafka_topic.orders: Refreshing state... [id=lkc-abc123/orders]
confluentcloud_kafka_acl.describe-orders: Refreshing state... [id=lkc-abc123/TOPIC#orders#LITERAL#User:sa-xyz123#*#DESCRIBE#ALLOW]
...
No changes. Infrastructure is up-to-date.
```

If you run into any problems, please [report an issue](https://github.com/confluentinc/terraform-provider-confluentcloud/issues).