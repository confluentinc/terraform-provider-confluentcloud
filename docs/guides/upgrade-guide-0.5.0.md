---
page_title: "Confluent Cloud Provider 0.5.0: Upgrade Guide"
---
# Confluent Cloud Provider 0.5.0: Upgrade Guide

This guide is intended to help with the upgrading process and focuses only on the changes necessary to upgrade to version `0.5.0` from `0.4.0` version.

-> **Note:** If you're currently using one of the earlier versions older than `0.4.0` please complete [Confluent Cloud Provider 0.4.0: Upgrade Guide](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/guides/upgrade-guide-0.4.0) before starting this one.

!> **Warning:** Don't forget to create a backup of `terraform.tfstate` state file before the upgrading. 

## Upgrade Notes

- [Provider Version Configuration](#provider-version-configuration)
- [Updating Terraform Configuration](#updating-terraform-configuration)
- [Upgrade State File](#upgrade-state-file)

## Provider Version Configuration

-> **Note:** This guide uses the Terraform configuration from [Sample Project](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/guides/sample-project) as an example of a Terraform configuration that has a cluster and 2 ACLs.

Before upgrading to version `0.5.0`, ensure that your environment
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

The next step is to set the latest version `0.5.0` in a `required_providers` block of your Terraform configuration and run `terraform init` to download the new version.

The existing configuration:
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

The updated configuration:
```hcl
terraform {
  required_providers {
   # ...
   confluentcloud = {
      source  = "confluentinc/confluentcloud"
      version = "0.5.0"
    }
  }
}
```

## Updating Terraform Configuration

### Changes to `confluentcloud_kafka_cluster` resource
New `api_version`, `kind` attributes were introduced to `confluentcloud_kafka_cluster` resource. No actions required.

### Changes to `confluentcloud_service_account` resource
New `api_version`, `kind` attributes were introduced to `confluentcloud_service_account` resource. No actions required.

## Upgrade State File
-> **Note:** We recommend you to use Terraform version manager [tfutils/tfenv](https://github.com/tfutils/tfenv): `tfenv install 0.14.0`, `tfenv use 0.14.0`.

Run `terraform refresh` command to fetch the values for new `api_version`, `kind` attributes.

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

##### Before the Upgrade
```
$ cat terraform.tfstate | grep -E '\"api_version\"|\"kind\"' # 0 matches
```
##### After the Upgrade
```
$ cat terraform.tfstate | grep -E '\"api_version\"|\"kind\"'
            "api_version": "cmk/v2",
            "kind": "Cluster",
            "api_version": "iam/v2",
            "kind": "ServiceAccount"
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