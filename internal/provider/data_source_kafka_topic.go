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
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
)

func kafkaTopicDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: kafkaTopicDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramClusterId: {
				Type:     schema.TypeString,
				Required: true,
			},
			paramTopicName: {
				Type:     schema.TypeString,
				Required: true,
			},
			paramHttpEndpoint: {
				Type:     schema.TypeString,
				Required: true,
			},
			paramCredentials: credentialsSchema(),
			paramPartitionsCount: {
				Type:     schema.TypeInt,
				Computed: true,
			},
			paramConfigs: {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
		},
	}
}

func kafkaTopicDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	httpEndpoint := d.Get(paramHttpEndpoint).(string)
	clusterId := d.Get(paramClusterId).(string)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	kafkaRestClient := meta.(*Client).kafkaRestClientFactory.CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret)
	topicName := d.Get(paramTopicName).(string)
	log.Printf("[INFO] Service account read for %s", topicName)

	err = executeKafkaTopicDataSourceRead(ctx, d, kafkaRestClient, topicName)

	return createDiagnosticsWithDetails(err)
}

// same as readAndSetTopicResourceConfigurationArguments but doesn't include resource deletion from TF state for 404
// and doesn't include credentials.
func executeKafkaTopicDataSourceRead(ctx context.Context, d *schema.ResourceData, c *KafkaRestClient, topicName string) error {
	kafkaTopic, resp, err := c.apiClient.TopicV3Api.GetKafkaV3Topic(c.apiContext(ctx), c.clusterId, topicName)
	if err != nil {
		log.Printf("[ERROR] Kafka topic get failed for id %s, %v, %s", topicName, resp, err)
		return err
	}
	if err := d.Set(paramClusterId, kafkaTopic.ClusterId); err != nil {
		return err
	}
	if err := d.Set(paramTopicName, kafkaTopic.TopicName); err != nil {
		return err
	}
	if err := d.Set(paramPartitionsCount, kafkaTopic.PartitionsCount); err != nil {
		return err
	}

	configs, err := loadTopicConfigs(ctx, d, c, topicName)
	if err != nil {
		return err
	}
	if err := d.Set(paramConfigs, configs); err != nil {
		return err
	}

	if err := d.Set(paramHttpEndpoint, c.httpEndpoint); err != nil {
		return err
	}
	kafkaTopicId := createKafkaTopicId(c.clusterId, topicName)
	d.SetId(kafkaTopicId)
	return nil
}
