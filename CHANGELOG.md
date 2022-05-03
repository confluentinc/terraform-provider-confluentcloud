## [Terraform Provider for Confluent Cloud](https://github.com/confluentinc/terraform-provider-confluentcloud) is deprecated in favor of [Terraform Provider for Confluent](https://github.com/confluentinc/terraform-provider-confluent)

## 0.6.0 (May 3, 2022)

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.5.0...v0.6.0)

* Deprecated the [Confluent Cloud Terraform Provider](https://github.com/confluentinc/terraform-provider-confluentcloud) in favor of the [Confluent Terraform Provider](https://github.com/confluentinc/terraform-provider-confluent).

## 0.5.0 (March 9, 2022)

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.4.0...v0.5.0)

* Added support for Kafka topic configuration updates ([#11](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/11)).
* Added support for `display_name` input for `confluentcloud_environment` and `confluentcloud_service_account` data sources ([#42](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/42), [#46](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/46)).
* Updated delete operation for `confluentcloud_kafka_topic` resource to avoid _400 Bad Request: Topic 'foobar' is marked for deletion_ error when recreating a lot of Kafka topics ([#50](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/50)).
* Fixed _Provider produced inconsistent result after apply_ error when creating a lot of Kafka topics ([#40](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/40)).
* Added support for old environment IDs ([#43](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/43)).
* Added `api_version` and `kind` computed attributes to `confluentcloud_kafka_cluster` and `confluentcloud_service_account` resources.
* Fixed docs issues.

## 0.4.0 (January 28, 2022)

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.3.0...v0.4.0)

* Added data sources ([#36](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/36)) for:
    * `confluentcloud_environment`
    * `confluentcloud_kafka_cluster`
    * `confluentcloud_kafka_topic`
    * `confluentcloud_service_account`
* Improved readability of error messages by adding details to them ([#28](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/28)).
* Resolved potential HTTP 429 errors by adding automatic retries with exponential backoff for HTTP requests ([#15](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/15), [#22](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/22)).
* Added graceful handling for resources created via Terraform but deleted via Confluent Cloud Console, Confluent CLI, or Confluent Cloud APIs.
* Fixed minor bugs and docs issues.

**Breaking changes**

* Removed a friction around manual look-up of IntegerID for Service Accounts by removing the need to use a `service_account_int_id` TF variable. If you are using the `confluentcloud_kafka_acl` resource you might see an input validation error after running `terraform plan`, which can be resolved by following [this guide](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/guides/upgrade-guide-0.4.0).
    * Updated "Sample project" [guide](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/guides/sample-project) to reflect this change.
* Simplified `confluentcloud_role_binding` resource creation by adding a new `rbac_crn` attribute for `confluentcloud_kafka_cluster` resource.
    * Updated the `confluentcloud_role_binding` resource examples to reflect this simplified approach.

## 0.3.0 (January 11, 2022)

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.2.0...v0.3.0)

* Added support for [role bindings](https://docs.confluent.io/cloud/current/api.html#tag/Role-Bindings-(iamv2)).
* Added support for rotating Cluster API Keys ([#21](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/21)).
* Updated SDK for [IAM APIs](https://docs.confluent.io/cloud/current/api.html#tag/Service-Accounts-(iamv2)) to use new routes.
* Resolved 2 Dependabot alerts.
* Fixed minor documentation issues.
* Moved from a closed Early Access to an open Preview for the Confluent Cloud Terraform Provider. All customers are now eligible to use the provider without explicit approval from the product team.

**Breaking changes**

* Early Access versions of the Confluent Cloud Terraform Provider (versions 0.1.0 and 0.2.0) are deprecated.

## 0.2.0 (November 5, 2021)

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.1.0...v0.2.0)

* Added support for dedicated Kafka clusters lifecycle management on a public network ([#5](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/5)).
* Added missing importers for `confluentcloud_kafka_topic` and `confluentcloud_kafka_acl` resources ([#7](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/7)).
* Fixed documentation issues ([#2](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/2), [#9](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/9), [#12](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/12)).

**Breaking changes**

* The format of `confluentcloud_kafka_acl` resource ID was updated, please [remove](https://www.terraform.io/docs/cli/commands/state/rm.html) its configuration from a TF state file and [reimport it](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/resources/confluentcloud_kafka_acl) when updating.

## 0.1.0 (October 1, 2021)

Initial Release
