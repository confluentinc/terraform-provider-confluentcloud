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
)

const (
	paramClusterId       = "kafka_cluster"
	paramTopicName       = "topic_name"
	paramCredentials     = "credentials"
	paramPartitionsCount = "partitions_count"
	paramKey             = "key"
	paramSecret          = "secret"
	paramConfigs         = "config"
)

func extractClusterId(d *schema.ResourceData) string {
	clusterId := d.Get(paramClusterId).(string)
	return clusterId
}
func extractTopicName(d *schema.ResourceData) string {
	topicName := d.Get(paramTopicName).(string)
	return topicName
}
func extractPartitionsCount(d *schema.ResourceData) int32 {
	partitionsCount := d.Get(paramPartitionsCount).(int)
	return int32(partitionsCount)
}

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

func extractHttpEndpoint(d *schema.ResourceData) string {
	httpEndpoint := d.Get(paramHttpEndpoint).(string)
	return httpEndpoint
}

func extractClusterApiKeyAndApiSecret(d *schema.ResourceData) (string, string, error) {
	credentialsDataList := d.Get(paramCredentials).(*schema.Set).List()
	if credentialsDataList == nil {
		return "", "", fmt.Errorf("expected a single %s block but found nil instead", paramCredentials)
	}
	if len(credentialsDataList) != 1 {
		return "", "", fmt.Errorf("expected a single %s block but found %d blocks instead", paramCredentials, len(credentialsDataList))
	}
	credentialsMap, ok := credentialsDataList[0].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("could not find cast %s block to map[string]interface{}", paramCredentials)
	}
	clusterApiKey, foundClusterApiKey := credentialsMap[paramKey]
	clusterApiSecret, foundClusterApiSecret := credentialsMap[paramSecret]
	if !foundClusterApiKey {
		return "", "", fmt.Errorf("could not find Kafka API Key for Confluent Cloud: there is no %s in %s block", paramKey, paramCredentials)
	} else if !foundClusterApiSecret {
		return "", "", fmt.Errorf("could not find Kafka API Secret for Confluent Cloud: there is no %s in %s block", paramSecret, paramCredentials)
	} else {
		return clusterApiKey.(string), clusterApiSecret.(string), nil
	}
}

func kafkaTopicResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: kafkaTopicCreate,
		ReadContext:   kafkaTopicRead,
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
	c := meta.(*Client)
	httpEndpoint := extractHttpEndpoint(d)
	updateKafkaRestClient(c, httpEndpoint)

	clusterId := extractClusterId(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	topicName := extractTopicName(d)

	kafkaTopicRequestData := kafkarestv3.CreateTopicRequestData{
		TopicName:       topicName,
		PartitionsCount: extractPartitionsCount(d),
		Configs:         extractConfigs(d),
	}

	// both replication_factor and partitions_count would be incorrectly set to 0 in a response so we don't use its data
	_, resp, err := executeKafkaTopicCreate(c.kafkaRestApiContext(ctx, clusterApiKey, clusterApiSecret), c, clusterId, kafkaTopicRequestData)

	if err != nil {
		log.Printf("[ERROR] Kafka topic create failed %v, %v, %s", kafkaTopicRequestData, resp, err)
		return diag.FromErr(err)
	}

	// issue another read request to fetch the real data
	kafkaTopicRead(ctx, d, meta)

	kafkaTopicId := createKafkaTopicId(clusterId, topicName)
	d.SetId(kafkaTopicId)
	log.Printf("[DEBUG] Created Kafka topic %s", kafkaTopicId)
	return nil
}

func executeKafkaTopicCreate(ctx context.Context, c *Client, clusterId string, requestData kafkarestv3.CreateTopicRequestData) (kafkarestv3.TopicData, *http.Response, error) {
	opts := &kafkarestv3.CreateKafkaV3TopicOpts{
		CreateTopicRequestData: optional.NewInterface(requestData),
	}
	return c.kafkaRestClient.TopicV3Api.CreateKafkaV3Topic(ctx, clusterId, opts)
}

func kafkaTopicDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka topic delete for %s", d.Id())

	c := meta.(*Client)
	httpEndpoint := extractHttpEndpoint(d)
	updateKafkaRestClient(c, httpEndpoint)

	clusterId := extractClusterId(d)
	topicName := extractTopicName(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}

	_, err = c.kafkaRestClient.TopicV3Api.DeleteKafkaV3Topic(c.kafkaRestApiContext(ctx, clusterApiKey, clusterApiSecret), clusterId, topicName)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting kafka topic (%s), err: %s", d.Id(), err))
	}

	return nil
}

func kafkaTopicRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka topic read for %s", d.Id())

	clusterId := extractClusterId(d)
	topicName := extractTopicName(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	httpEndpoint := extractHttpEndpoint(d)

	_, err = readAndSetTopicResourceConfigurationArguments(ctx, d, meta, clusterId, topicName, clusterApiKey, clusterApiSecret, httpEndpoint)

	return diag.FromErr(err)
}

func createKafkaTopicId(clusterId, topicName string) string {
	return fmt.Sprintf("%s/%s", clusterId, topicName)
}

func updateKafkaRestClient(c *Client, httpEndpoint string) {
	if c.kafkaRestClient.GetConfig().BasePath == httpEndpoint {
		return
	}
	kafkaRestConfig := kafkarestv3.NewConfiguration()
	kafkaRestConfig.BasePath = httpEndpoint
	c.kafkaRestClient = kafkarestv3.NewAPIClient(kafkaRestConfig)
}

func credentialsSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeSet,
		ForceNew:    true,
		Required:    true,
		Description: "The Cluster API Credentials.",
		MinItems:    1,
		MaxItems:    1,
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
		Description:  "The Kafka cluster ID (e.g., `cluster-1`).",
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

	return readAndSetTopicResourceConfigurationArguments(ctx, d, meta, clusterId, topicName, kafkaImportEnvVars.kafkaApiKey, kafkaImportEnvVars.kafkaApiSecret, kafkaImportEnvVars.kafkaHttpEndpoint)
}

func readAndSetTopicResourceConfigurationArguments(ctx context.Context, d *schema.ResourceData, meta interface{}, clusterId, topicName, kafkaApiKey, kafkaApiSecret, httpEndpoint string) ([]*schema.ResourceData, error) {
	c := meta.(*Client)
	updateKafkaRestClient(c, httpEndpoint)

	ctx = c.kafkaRestApiContext(ctx, kafkaApiKey, kafkaApiSecret)

	kafkaTopic, resp, err := c.kafkaRestClient.TopicV3Api.GetKafkaV3Topic(ctx, clusterId, topicName)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		// https://learn.hashicorp.com/tutorials/terraform/provider-setup?in=terraform/providers
		// If the resource isn't available, set the ID to an empty string so Terraform "destroys" the resource in state.
		d.SetId("")
		return nil, nil
	}
	if err != nil {
		log.Printf("[ERROR] Kafka topic get failed for id %s, %v, %s", topicName, resp, err)
	}
	if err == nil {
		err = d.Set(paramClusterId, kafkaTopic.ClusterId)
	}
	if err == nil {
		err = d.Set(paramTopicName, kafkaTopic.TopicName)
	}
	if err == nil {
		err = d.Set(paramPartitionsCount, kafkaTopic.PartitionsCount)
	}
	if err == nil {
		topicConfigList, resp, err := c.kafkaRestClient.ConfigsV3Api.ListKafkaV3TopicConfigs(ctx, clusterId, topicName)
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
		err = d.Set(paramConfigs, config)
		if err != nil {
			return nil, err
		}
	}

	if err == nil {
		err = setKafkaCredentials(kafkaApiKey, kafkaApiSecret, d)
	}
	if err == nil {
		err = d.Set(paramHttpEndpoint, httpEndpoint)
	}
	return []*schema.ResourceData{d}, err
}

func setKafkaCredentials(kafkaApiKey, kafkaApiSecret string, d *schema.ResourceData) error {
	return d.Set(paramCredentials, []interface{}{map[string]interface{}{
		paramKey:    kafkaApiKey,
		paramSecret: kafkaApiSecret,
	}})
}
