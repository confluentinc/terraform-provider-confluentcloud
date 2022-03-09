// Copyright 2021 Confluent Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	kafkarestv3 "github.com/confluentinc/ccloud-sdk-go-v2/kafkarest/v3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	paramClusterId              = "kafka_cluster"
	paramTopicName              = "topic_name"
	paramCredentials            = "credentials"
	paramPartitionsCount        = "partitions_count"
	paramKey                    = "key"
	paramSecret                 = "secret"
	paramConfigs                = "config"
	kafkaRestAPIWaitAfterCreate = 10 * time.Second
	docsUrl                     = "https://registry.terraform.io/providers/confluentinc/confluentcloud/latest/docs/resources/confluentcloud_kafka_topic"
)

// https://docs.confluent.io/cloud/current/clusters/broker-config.html#custom-topic-settings-for-all-cluster-types
var editableTopicSettings = []string{"delete.retention.ms", "max.message.bytes", "max.compaction.lag.ms",
	"message.timestamp.difference.max.ms", "message.timestamp.type", "min.compaction.lag.ms", "min.insync.replicas",
	"retention.bytes", "retention.ms", "segment.bytes", "segment.ms"}

func extractConfigs(configs map[string]interface{}) []kafkarestv3.CreateTopicRequestDataConfigs {
	configResult := make([]kafkarestv3.CreateTopicRequestDataConfigs, len(configs))

	i := 0
	for name, value := range configs {
		v := value.(string)
		configResult[i] = kafkarestv3.CreateTopicRequestDataConfigs{
			Name:  name,
			Value: &v,
		}
		i += 1
	}

	return configResult
}

func extractClusterApiKeyAndApiSecret(d *schema.ResourceData) (string, string, error) {
	clusterApiKey, err := extractStringAttributeFromListBlockOfSizeOne(d, paramCredentials, paramKey)
	if err != nil {
		return "", "", err
	}
	clusterApiSecret, err := extractStringAttributeFromListBlockOfSizeOne(d, paramCredentials, paramSecret)
	if err != nil {
		return "", "", err
	}
	return clusterApiKey, clusterApiSecret, nil
}

func kafkaTopicResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: kafkaTopicCreate,
		ReadContext:   kafkaTopicRead,
		UpdateContext: kafkaTopicUpdate,
		DeleteContext: kafkaTopicDelete,
		Importer: &schema.ResourceImporter{
			StateContext: kafkaTopicImport,
		},
		Schema: map[string]*schema.Schema{
			paramClusterId: clusterIdSchema(),
			paramTopicName: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The name of the topic.",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			paramPartitionsCount: {
				Type:        schema.TypeInt,
				Optional:    true,
				Default:     6,
				ForceNew:    true,
				Description: "The number of partitions to create in the topic.",
			},
			paramHttpEndpoint: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The REST endpoint of the Kafka cluster.",
			},
			paramConfigs: {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:    true,
				Description: "The custom topic settings to set (e.g., `\"cleanup.policy\" = \"compact\"`).",
			},
			paramCredentials: credentialsSchema(),
		},
	}
}

func kafkaTopicCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	httpEndpoint := d.Get(paramHttpEndpoint).(string)
	clusterId := d.Get(paramClusterId).(string)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret)
	topicName := d.Get(paramTopicName).(string)

	kafkaTopicRequestData := kafkarestv3.CreateTopicRequestData{
		TopicName:       topicName,
		PartitionsCount: int32(d.Get(paramPartitionsCount).(int)),
		Configs:         extractConfigs(d.Get(paramConfigs).(map[string]interface{})),
	}

	_, resp, err := executeKafkaTopicCreate(ctx, kafkaRestClient, kafkaTopicRequestData)

	if err != nil {
		log.Printf("[ERROR] Kafka topic create failed %v, %v, %s", kafkaTopicRequestData, resp, err)
		return createDiagnosticsWithDetails(err)
	}

	kafkaTopicId := createKafkaTopicId(kafkaRestClient.clusterId, topicName)
	d.SetId(kafkaTopicId)
	log.Printf("[DEBUG] Created Kafka topic %s", kafkaTopicId)

	// https://github.com/confluentinc/terraform-provider-confluentcloud/issues/40#issuecomment-1048782379
	time.Sleep(kafkaRestAPIWaitAfterCreate)

	return kafkaTopicRead(ctx, d, meta)
}

func executeKafkaTopicCreate(ctx context.Context, c *KafkaRestClient, requestData kafkarestv3.CreateTopicRequestData) (kafkarestv3.TopicData, *http.Response, error) {
	opts := &kafkarestv3.CreateKafkaV3TopicOpts{
		CreateTopicRequestData: optional.NewInterface(requestData),
	}
	return c.apiClient.TopicV3Api.CreateKafkaV3Topic(c.apiContext(ctx), c.clusterId, opts)
}

func kafkaTopicDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka topic delete for %s", d.Id())

	httpEndpoint := d.Get(paramHttpEndpoint).(string)
	clusterId := d.Get(paramClusterId).(string)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret)
	topicName := d.Get(paramTopicName).(string)

	_, err = kafkaRestClient.apiClient.TopicV3Api.DeleteKafkaV3Topic(kafkaRestClient.apiContext(ctx), kafkaRestClient.clusterId, topicName)

	if err != nil {
		return diag.Errorf("error deleting kafka topic (%s), err: %s", d.Id(), err)
	}

	if err := waitForKafkaTopicToBeDeleted(kafkaRestClient.apiContext(ctx), kafkaRestClient, topicName); err != nil {
		return diag.Errorf("error waiting for Kafka topic (%s) to be deleted, err: %s", d.Id(), err)
	}

	log.Printf("[INFO] Kafka topic %s was deleted successfully", d.Id())

	return nil
}

func kafkaTopicRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka topic read for %s", d.Id())

	httpEndpoint := d.Get(paramHttpEndpoint).(string)
	clusterId := d.Get(paramClusterId).(string)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret)
	topicName := d.Get(paramTopicName).(string)

	_, err = readAndSetTopicResourceConfigurationArguments(ctx, d, kafkaRestClient, topicName)

	return createDiagnosticsWithDetails(err)
}

func createKafkaTopicId(clusterId, topicName string) string {
	return fmt.Sprintf("%s/%s", clusterId, topicName)
}

func credentialsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Required:    true,
		Description: "The Cluster API Credentials.",
		MinItems:    1,
		MaxItems:    1,
		Sensitive:   true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramKey: {
					Type:         schema.TypeString,
					Required:     true,
					Description:  "The Cluster API Key for your Confluent Cloud cluster.",
					Sensitive:    true,
					ValidateFunc: validation.StringIsNotEmpty,
				},
				paramSecret: {
					Type:         schema.TypeString,
					Required:     true,
					Description:  "The Cluster API Secret for your Confluent Cloud cluster.",
					Sensitive:    true,
					ValidateFunc: validation.StringIsNotEmpty,
				},
			},
		},
	}
}

func clusterIdSchema() *schema.Schema {
	return &schema.Schema{
		Type:         schema.TypeString,
		Required:     true,
		ForceNew:     true,
		Description:  "The Kafka cluster ID (e.g., `lkc-12345`).",
		ValidateFunc: validation.StringMatch(regexp.MustCompile("^lkc-"), "the Kafka cluster ID must be of the form 'lkc-'"),
	}
}

func kafkaTopicImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	log.Printf("[INFO] Kafka topic import for %s", d.Id())

	kafkaImportEnvVars, err := checkEnvironmentVariablesForKafkaImportAreSet()
	if err != nil {
		return nil, err
	}

	clusterIDAndTopicName := d.Id()
	parts := strings.Split(clusterIDAndTopicName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for kafka topic import: expected '<lkc ID>/<topic name>'")
	}

	clusterId := parts[0]
	topicName := parts[1]

	kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(kafkaImportEnvVars.kafkaHttpEndpoint, clusterId, kafkaImportEnvVars.kafkaApiKey, kafkaImportEnvVars.kafkaApiSecret)

	return readAndSetTopicResourceConfigurationArguments(ctx, d, kafkaRestClient, topicName)
}

func readAndSetTopicResourceConfigurationArguments(ctx context.Context, d *schema.ResourceData, c *KafkaRestClient, topicName string) ([]*schema.ResourceData, error) {
	kafkaTopic, resp, err := c.apiClient.TopicV3Api.GetKafkaV3Topic(c.apiContext(ctx), c.clusterId, topicName)
	if err != nil {
		log.Printf("[WARN] Kafka topic get failed for id %s, %v, %s", topicName, resp, err)

		// https://learn.hashicorp.com/tutorials/terraform/provider-setup
		isResourceNotFound := HasStatusNotFound(resp)
		if isResourceNotFound && !d.IsNewResource() {
			log.Printf("[WARN] Kafka topic with id=%s is not found", d.Id())
			// If the resource isn't available, Terraform destroys the resource in state.
			d.SetId("")
			return nil, nil
		}

		return nil, err
	}
	if err := d.Set(paramClusterId, kafkaTopic.ClusterId); err != nil {
		return nil, err
	}
	if err := d.Set(paramTopicName, kafkaTopic.TopicName); err != nil {
		return nil, err
	}
	if err := d.Set(paramPartitionsCount, kafkaTopic.PartitionsCount); err != nil {
		return nil, err
	}

	configs, err := loadTopicConfigs(ctx, d, c, topicName)
	if err != nil {
		return nil, err
	}
	if err := d.Set(paramConfigs, configs); err != nil {
		return nil, err
	}

	if err := setKafkaCredentials(c.clusterApiKey, c.clusterApiSecret, d); err != nil {
		return nil, err
	}
	if err := d.Set(paramHttpEndpoint, c.httpEndpoint); err != nil {
		return nil, err
	}
	return []*schema.ResourceData{d}, err
}

func kafkaTopicUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChangesExcept(paramCredentials, paramConfigs) {
		return diag.Errorf("only %s and %s blocks can be updated for a Kafka topic", paramCredentials, paramConfigs)
	}
	if d.HasChange(paramConfigs) {
		log.Printf("[INFO] Kafka Topic config update for '%s'", d.Get(paramTopicName).(string))

		// TF Provider allows the following operations for editable topic settings under 'config' block:
		// 1. Adding new key value pair, for example, "retention.ms" = "600000"
		// 2. Update a value for existing key value pair, for example, "retention.ms" = "600000" -> "retention.ms" = "600001"
		// You might find the list of editable topic settings and their limits at
		// https://docs.confluent.io/cloud/current/clusters/broker-config.html#custom-topic-settings-for-all-cluster-types

		// Extract 'old' and 'new' (include changes in TF configuration) topic settings
		// * 'old' topic settings -- all topic settings from TF configuration _before_ changes / updates (currently set on Confluent Cloud)
		// * 'new' topic settings -- all topic settings from TF configuration _after_ changes
		oldTopicSettingsMap, newTopicSettingsMap := extractOldAndNewTopicSettings(d)

		// Verify that no topic settings were removed (reset to its default value) in TF configuration which is an unsupported operation at the moment
		for oldTopicSettingName := range oldTopicSettingsMap {
			if _, ok := newTopicSettingsMap[oldTopicSettingName]; !ok {
				return diag.Errorf("Reset to topic setting's default value operation (in other words, removing topic settings from 'configs' block) "+
					"is not supported at the moment. "+
					"Instead, find its default value at %s and set its current value to the default value.", docsUrl)
			}
		}

		// Store only topic settings that were updated in TF configuration.
		// Will be used for creating a request to Kafka REST API.
		var topicSettingsUpdateBatch []kafkarestv3.AlterConfigBatchRequestDataData

		// Verify that topics that were changed in TF configuration settings are indeed editable
		for topicSettingName, newTopicSettingValue := range newTopicSettingsMap {
			oldTopicSettingValue, ok := oldTopicSettingsMap[topicSettingName]
			isTopicSettingValueUpdated := !(ok && oldTopicSettingValue == newTopicSettingValue)
			if isTopicSettingValueUpdated {
				// operation #1 (ok = False) or operation #2 (ok = True, oldTopicSettingValue != newTopicSettingValue)
				isTopicSettingEditable := stringInSlice(topicSettingName, editableTopicSettings)
				if isTopicSettingEditable {
					topicSettingsUpdateBatch = append(topicSettingsUpdateBatch, kafkarestv3.AlterConfigBatchRequestDataData{
						Name:  topicSettingName,
						Value: ptr(newTopicSettingValue),
					})
				} else {
					return diag.Errorf("'%s' topic setting cannot be updated since it is read-only. "+
						"Read %s for more details.", topicSettingName, docsUrl)
				}
			}
		}

		// Construct a request for Kafka REST API
		requestData := kafkarestv3.AlterConfigBatchRequestData{
			Data: topicSettingsUpdateBatch,
		}
		httpEndpoint := d.Get(paramHttpEndpoint).(string)
		clusterId := d.Get(paramClusterId).(string)
		clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
		if err != nil {
			return createDiagnosticsWithDetails(err)
		}
		kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret)
		topicName := d.Get(paramTopicName).(string)

		// Send a request to Kafka REST API
		_, err = executeKafkaTopicUpdate(ctx, kafkaRestClient, topicName, requestData)
		if err != nil {
			// For example, Kafka REST API will return Bad Request if new topic setting value exceeds the max limit:
			// 400 Bad Request: Config property 'delete.retention.ms' with value '63113904003' exceeded max limit of 60566400000.
			return createDiagnosticsWithDetails(err)
		}
		// Give some time to Kafka REST API to apply an update of topic settings
		time.Sleep(kafkaRestAPIWaitAfterCreate)

		// Check that topic configs update was successfully executed
		// In other words, remote topic setting values returned by Kafka REST API match topic setting values from updated TF configuration
		actualTopicSettings, err := loadTopicConfigs(ctx, d, kafkaRestClient, topicName)
		if err != nil {
			return createDiagnosticsWithDetails(err)
		}

		var updatedTopicSettings, outdatedTopicSettings []string
		for _, v := range topicSettingsUpdateBatch {
			if v.Value == nil {
				// It will never happen because of the way we construct topicSettingsUpdateBatch
				continue
			}
			topicSettingName := v.Name
			expectedValue := *v.Value
			actualValue, ok := actualTopicSettings[topicSettingName]
			if ok && actualValue != expectedValue {
				outdatedTopicSettings = append(outdatedTopicSettings, topicSettingName)
			} else {
				updatedTopicSettings = append(updatedTopicSettings, topicSettingName)
			}
		}
		if len(outdatedTopicSettings) > 0 {
			return diag.Errorf("Update failed for the following topic settings: %v. "+
				"Double check that these topic settings are indeed editable and provided target values do not exceed min/max allowed values by reading %s", outdatedTopicSettings, docsUrl)
		}
		log.Printf("[INFO] Kafka Topic config update for '%s' topic was completed successfully for the following topic settings: %v", topicName, updatedTopicSettings)
	}
	return nil
}

func executeKafkaTopicUpdate(ctx context.Context, c *KafkaRestClient, topicName string, requestData kafkarestv3.AlterConfigBatchRequestData) (*http.Response, error) {
	opts := &kafkarestv3.UpdateKafkaV3TopicConfigBatchOpts{
		AlterConfigBatchRequestData: optional.NewInterface(requestData),
	}
	return c.apiClient.ConfigsV3Api.UpdateKafkaV3TopicConfigBatch(c.apiContext(ctx), c.clusterId, topicName, opts)
}

func setKafkaCredentials(kafkaApiKey, kafkaApiSecret string, d *schema.ResourceData) error {
	return d.Set(paramCredentials, []interface{}{map[string]interface{}{
		paramKey:    kafkaApiKey,
		paramSecret: kafkaApiSecret,
	}})
}

func loadTopicConfigs(ctx context.Context, d *schema.ResourceData, c *KafkaRestClient, topicName string) (map[string]string, error) {
	topicConfigList, resp, err := c.apiClient.ConfigsV3Api.ListKafkaV3TopicConfigs(c.apiContext(ctx), c.clusterId, topicName)
	if err != nil {
		log.Printf("[ERROR] Kafka topic config get failed for id %s, %v, %s", d.Id(), resp, err)
		return nil, err
	}

	config := make(map[string]string)
	for _, remoteConfig := range topicConfigList.Data {
		// Extract configs that were set via terraform vs set by default
		if remoteConfig.Source == kafkarestv3.CONFIGSOURCE_DYNAMIC_TOPIC_CONFIG && remoteConfig.Value != nil {
			config[remoteConfig.Name] = *remoteConfig.Value
		}
	}
	return config, nil
}

func extractOldAndNewTopicSettings(d *schema.ResourceData) (map[string]string, map[string]string) {
	oldConfigs, newConfigs := d.GetChange(paramConfigs)
	return convertToStringStringMap(oldConfigs.(map[string]interface{})), convertToStringStringMap(newConfigs.(map[string]interface{}))
}
