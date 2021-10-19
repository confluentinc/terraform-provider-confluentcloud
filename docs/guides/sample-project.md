---
page_title: "Sample Project"
---

# Sample Project for Confluent Cloud Terraform Provider

Use the Confluent Cloud Terraform provider to automate the workflow for creating
a _Service Account_, a _Confluent Cloud environment_, a _Kafka cluster_, and
_Topics_. Also, you can use this provider to assign permissions (_ACLs_) that
enable access to the topics you create.

In this guide, you will:

1. [Get a Confluent Cloud API Key](#get-a-confluent-cloud-api-key)
2. [Run Terraform to create your Kafka cluster](#run-terraform-to-create-your-kafka-cluster)
3. [Inspect your automatically created resources](#inspect-your-resources)
4. [Run Terraform to create a Kafka topic and ACLs](#run-terraform-to-create-a-kafka-topic)
5. [Clean up and delete resources](#clean-up-and-delete-resources)

## Prerequisites

- A Confluent Cloud account. **[Sign up here](https://www.confluent.io/get-started?product=cloud)**
- To run this example, you must be enrolled into the Confluent Cloud API v2 API Early Access program. To enroll,
  [email Confluent support](mailto:cflt-tf-access@confluent.io?subject=Request%20to%20join%20v2%20API%20Early%20Access&body=I%E2%80%99d%20like%20to%20join%20the%20Confluent%20Cloud%20API%20Early%20Access%20for%20v2%20to%20provide%20early%20feedback%21%20My%20Cloud%20Organization%20ID%20is%20%3Cretrieve%20from%20https%3A//confluent.cloud/settings/billing/payment%3E.). If you're not
  enrolled, you'll receive `403` errors when you apply Terraform plans.

## Get a Confluent Cloud API Key

The following steps show how to get the Confluent Cloud API key and secret that
you need to access Confluent Cloud programmatically.

-> **Note:** When you create the Cloud API key, you must select **Global access**.

1.  Create a Cloud API key and secret by using the
    [Confluent Cloud Console](https://confluent.cloud/settings/api-keys) or the
    [Confluent Cloud CLI](https://docs.confluent.io/ccloud-cli/current/command-reference/api-key/ccloud_api-key_create.html).
    They're required for creating any Confluent Cloud resources.

    If you're using the Confluent Cloud CLI, the following command creates your
    API key and secret.

    ```bash
    ccloud api-key create --resource "cloud" 
    ```

    Save your API key and secret in a secure location.

2.  Run the following commands to set the `CONFLUENT_CLOUD_API_KEY` and
    `CONFLUENT_CLOUD_API_SECRET` environment variables:

    ```bash
    export CONFLUENT_CLOUD_API_KEY="<cloud_api_key>"
    export CONFLUENT_CLOUD_API_SECRET="<cloud_api_secret>"
    ```
    
    -> **Note:** Quotation marks are required around the API key and secret strings.

    The provider uses these environment variables to authenticate to
    Confluent Cloud.

## Run Terraform to create your Kafka cluster 

Run Terraform to create a service account and an environment that has a
Kafka cluster.

1.  Create a new file named `main.tf` and copy the following Terraform template
    into it.

    ```terraform

    # Example for using Confluent Cloud https://docs.confluent.io/cloud/current/api.html
    # that creates multiple resources: a service account, an environment, a basic cluster, a topic, and 2 ACLs.
    # Configure Confluent Cloud provider
    terraform {
      required_providers {
        confluentcloud = {
          source  = "confluentinc/confluentcloud"
          version = "0.1.0"
        }
      }
    }
    
    provider "confluentcloud" {}

    resource "confluentcloud_service_account" "test-sa" {
      display_name = "test_sa"
      description = "description for test_sa"
    }

    resource "confluentcloud_environment" "test-env" {
      display_name = "test_env"
    }

    resource "confluentcloud_kafka_cluster" "test-basic-cluster" {
      display_name = "test_cluster"
      availability = "SINGLE_ZONE"
      cloud = "GCP"
      region = "us-central1"
      basic {}
      environment {
        id = confluentcloud_environment.test-env.id
      }
    }
    ```

2.  Run the following command to initialize the Confluent Cloud Terraform provider:

    ```bash
    terraform init
    ```

3.  Run the following command to create the plan:

    ```bash
    terraform plan -parallelism=1 -out=tfplan_add_sa_env_and_cluster
    ```

4.  Run the following command to apply the plan and create cloud resources:

    ```bash
    terraform apply -parallelism=1 tfplan_add_sa_env_and_cluster
    ```

    Your output should resemble:

    ```
    confluentcloud_service_account.test-sa: Creating...
    confluentcloud_environment.test-env: Creating...
    confluentcloud_environment.test-env: Creation complete after 1s [id=env-***] <--- the Environment's ID
    confluentcloud_kafka_cluster.test-basic-cluster: Creating...
    confluentcloud_service_account.test-sa: Creation complete after 1s [id=sa-***] <--- the Service Account's ID
    confluentcloud_kafka_cluster.test-basic-cluster: Still creating... [10s elapsed]
    confluentcloud_kafka_cluster.test-basic-cluster: Creation complete after 14s [id=lkc-***] <--- the Kafka cluster's ID

    Apply complete! Resources: 3 added, 0 changed, 0 destroyed.
    ```

    Terraform creates a `test_sa` service account and a `test_env` environment
    that has a Kafka cluster, named `test_cluster`.

## Inspect your resources

You can find the created resources (and their IDs: `sa-***`, `env-***`, `lkc-***` 
from Terraform output) on both [Cloud Console](https://confluent.cloud/environments) and
[Confluent Cloud CLI](https://docs.confluent.io/ccloud-cli/current/index.html):

The following steps show how to get the integer identifier for the `confluentcloud_service_account` resource that you'll use in later steps.  

### Use the Confluent Cloud Console to inspect your resources

The following steps show how to get the integer identifier for the `confluentcloud_service_account` resource by using
the Cloud Console.

1.  Log in to your [Confluent Cloud account](https://confluent.cloud/login).

2.  Open the [Accounts and access tab](https://confluent.cloud/settings/org/accounts/service-accounts)
    and click **test-sa**. Copy the integer from the URL. For example, 
    in this URL, `https://confluent.cloud/settings/org/accounts/service-accounts/309715/settings`, 
    the ID is `309715`.

3.  Save the service account ID.

### Use the Confluent Cloud CLI to inspect your resources

Use the following `ccloud` commands to get the integer identifier for the `confluentcloud_service_account` resource by using the
Cloud CLI. 

1.  Run the following command to login:

    ```bash
    ccloud login
    ```

2.  Run the following command to find the service account:

    ```bash
    ccloud service-account list | grep 'Resource ID\|-+-\|test_sa'
    ```

    Your output should resemble:

    ```
        Id   | Resource ID |  Name   |       Description
    +--------+-------------+---------+-------------------------+
      309715 | sa-l7v772   | test_sa | description for test_sa
    ```

    The **Id** column shows the identifier for the service account. Save the
    service account ID in a secure location.
    
    -> **Note:** We won’t expose integer IDs for the service account in v2.0 of the Cloud CLI.

## Run Terraform to create a Kafka topic

In previous steps, you used the Confluent Cloud Provider to create a Service Account, an
environment, and a Kafka cluster, but your cluster doesn't have any topics.
In this step, you create an API key to access the Kafka cluster, and you use
it in a plan to create a Kafka topic and related ACLs that authorize access.

-> **Important** You must manually provision API keys for the service account so it can authenticate with the cluster. ACL management covers only authorization, not authentication and is a manual step after the creation of the Kafka cluster.

The following steps show how to create a Kafka API key and use it in a 
Terraform file to create a topic and its related ACLs.

Create an API key and secret for the Kafka cluster by using the
[Confluent Cloud Console](https://confluent.cloud/settings/api-keys) or the
[Confluent Cloud CLI](https://docs.confluent.io/ccloud-cli/current/command-reference/api-key/ccloud_api-key_create.html).
The Kafka API key is distinct from the Cloud API key and is required for
creating Kafka topics and ACLs.

### Use the Cloud Console to get your Kafka API key

The following steps show how to create a Kafka API key by using the Cloud Console.

1.  Click into the **test_env** environment and then click **test_cluster**.

2.  In the navigation menu on the left, click **Data integration** and click **API keys**.

3.  Click **Add key**, and in the Create Key page, click **Global access** and
    **Next**. Your API key is generated and displayed for easy copying.
4.  Copy and save your API key and secret in a secure location. When you're
    done, click **I have saved my API key** and click **Save**.

### Use the Cloud CLI to get your Kafka API key

If you're using the Confluent Cloud CLI, the following command creates your
API key and secret. Replace `<cluster_id>` and `<env_id>` with your cluster ID and environment ID respectively.

```bash
ccloud api-key create --resource <cluster_id> --environment <env_id> 
```

Save your Kafka API key and secret in a secure location.

### Add your Kafka API key to a Terraform file

1.  Create a new file named `variables.tf` and copy the following template into
    it. In each `variable` block, set `default` to the corresponding property value.
    `service_account_int_id` variables.

    ```terraform

    variable "kafka_api_key" {
      type = string
      description = "Kafka API Key"
    }

    variable "kafka_api_secret" {
      type = string
      description = "Kafka API Secret"
      sensitive = true  
    }

    variable "service_account_int_id" {
      type = number
      description = "Service Account Integer ID"
    }
    ```

2.  Create a new file named `terraform.tfvars` and copy the following into it.
    Copy the below template and substitute the correct values for each variable.

    -> **Important:** Do not store production secrets in a `.tfvars` file. Instead,
    [use environment variables, encrypted files, or a secret store](https://blog.gruntwork.io/a-comprehensive-guide-to-managing-secrets-in-your-terraform-code-1d586955ace1)

    ```terraform
    kafka_api_key="<key>"
    kafka_api_secret="<secret>"
    service_account_int_id=<id>
    ```

    -> **Note:** Quotation marks are required around the API key and secret strings.

3.  Append the following resource definitions into `main.tf` and save the file.

    ```terraform
    resource "confluentcloud_kafka_topic" "orders" {
      kafka_cluster = confluentcloud_kafka_cluster.test-basic-cluster.id
      topic_name = "orders"
      partitions_count = 4
      http_endpoint = confluentcloud_kafka_cluster.test-basic-cluster.http_endpoint
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
    
    resource "confluentcloud_kafka_acl" "describe-orders" {
      kafka_cluster = confluentcloud_kafka_cluster.test-basic-cluster.id
      resource_type = "TOPIC"
      resource_name = confluentcloud_kafka_topic.orders.topic_name
      pattern_type = "LITERAL"
      principal = "User:${var.service_account_int_id}"
      operation = "DESCRIBE"
      permission = "ALLOW"
      http_endpoint = confluentcloud_kafka_cluster.test-basic-cluster.http_endpoint
      credentials {
        key = var.kafka_api_key
        secret = var.kafka_api_secret
      }
    }
    
    resource "confluentcloud_kafka_acl" "describe-test-basic-cluster" {
      kafka_cluster = confluentcloud_kafka_cluster.test-basic-cluster.id
      resource_type = "CLUSTER"
      resource_name = "kafka-cluster"
      pattern_type = "LITERAL"
      principal = "User:${var.service_account_int_id}"
      operation = "DESCRIBE"
      permission = "ALLOW"
      http_endpoint = confluentcloud_kafka_cluster.test-basic-cluster.http_endpoint
      credentials {
        key = var.kafka_api_key
        secret = var.kafka_api_secret
      }
    }
    ```

4.  Run the following command to create the plan.

    ```bash
    terraform plan -parallelism=1 -out=tfplan_add_topic_and_2_acls
    ```

    Your output should resemble:

5.  Run the following command to apply the plan.
    
    ```bash
    terraform apply -parallelism=1 tfplan_add_topic_and_2_acls
    ```

    Your output should resemble:

    ```
    confluentcloud_kafka_acl.describe-test-basic-cluster: Creating...
    confluentcloud_kafka_topic.orders: Creating...
    confluentcloud_kafka_acl.describe-test-basic-cluster: Creation complete after 1s [id=lkc-odgpo/CLUSTER/kafka-cluster/LITERAL/User:309715/*/DESCRIBE/ALLOW]
    confluentcloud_kafka_topic.orders: Creation complete after 2s [id=lkc-odgpo/orders]
    confluentcloud_kafka_acl.describe-orders: Creating...
    confluentcloud_kafka_acl.describe-orders: Creation complete after 0s [id=lkc-odgpo/TOPIC/orders/LITERAL/User:309715/*/DESCRIBE/ALLOW]

    Apply complete! Resources: 3 added, 0 changed, 0 destroyed.
    ```

### Clean up and delete resources

To clean up and remove the resources you've created, run the following command:

```bash
terraform destroy -parallelism=1 --auto-approve
```

Your output should resemble:

```
confluentcloud_service_account.test-sa: Destroying... [id=sa-l7v772]
confluentcloud_kafka_acl.describe-orders: Destroying... [id=lkc-odgpo/TOPIC/orders/LITERAL/User:309715/*/DESCRIBE/ALLOW]
confluentcloud_kafka_acl.describe-test-basic-cluster: Destroying... [id=lkc-odgpo/CLUSTER/kafka-cluster/LITERAL/User:309715/*/DESCRIBE/ALLOW]
confluentcloud_kafka_acl.describe-orders: Destruction complete after 2s
confluentcloud_kafka_acl.describe-test-basic-cluster: Destruction complete after 2s
confluentcloud_kafka_topic.orders: Destroying... [id=lkc-odgpo/orders]
confluentcloud_service_account.test-sa: Destruction complete after 2s
confluentcloud_kafka_topic.orders: Destruction complete after 0s
confluentcloud_kafka_cluster.test-basic-cluster: Destroying... [id=lkc-odgpo]
confluentcloud_kafka_cluster.test-basic-cluster: Still destroying... [id=lkc-odgpo, 10s elapsed]
confluentcloud_kafka_cluster.test-basic-cluster: Still destroying... [id=lkc-odgpo, 20s elapsed]
confluentcloud_kafka_cluster.test-basic-cluster: Destruction complete after 23s
confluentcloud_environment.test-env: Destroying... [id=env-31dgj]
confluentcloud_environment.test-env: Destroying... [id=env-31dgj]
confluentcloud_environment.test-env: Destruction complete after 1s

Apply complete! Resources: 0 added, 0 changed, 7 destroyed.
```

-> **Next steps:** Explore examples in the [Confluent Cloud Provider repo](https://github.com/confluentinc/terraform-provider-confluentcloud/tree/master/examples)
