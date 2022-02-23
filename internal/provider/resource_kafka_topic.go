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
)

func extractConfigs(d *schema.ResourceData) []kafkarestv3.CreateTopicRequestDataConfigs {
	configs := d.Get(paramConfigs).(map[string]interface{})

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
				ForceNew:    true,
				Description: "The custom topic configurations to set (e.g., `\"cleanup.policy\" = \"compact\"`).",
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
		Configs:         extractConfigs(d),
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
		if isResourceNotFound {
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
	if d.HasChangesExcept(paramCredentials) {
		return diag.Errorf("only %s block can be updated for a Kafka topic", paramCredentials)
	}
	return nil
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
