## 0.2.0

[Full Changelog](https://github.com/confluentinc/terraform-provider-confluentcloud/compare/v0.1.0...v0.2.0)

* Added support for dedicated Kafka clusters lifecycle management on a public network ([#5](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/5)).
* Added missing importers for `confluentcloud_kafka_topic` and `confluentcloud_kafka_acl` resources ([#7](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/7)).
* Fixed documentation issues ([#2](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/2), [#9](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/9), [#12](https://github.com/confluentinc/terraform-provider-confluentcloud/issues/12)).

**Breaking changes**

* The format of `confluentcloud_kafka_acl` resource ID was updated, please [remove](https://www.terraform.io/docs/cli/commands/state/rm.html) its configuration from a TF state file and [reimport it](https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/resources/confluentcloud_kafka_acl) when updating.

## 0.1.0 (October 1, 2021)

Initial Release
