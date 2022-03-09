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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"log"
	"strings"
	"time"
)

func waitForKafkaClusterToProvision(ctx context.Context, c *Client, environmentId, clusterId, clusterType string) error {
	stateConf := &resource.StateChangeConf{
		Pending:      []string{stateInProgress},
		Target:       []string{stateDone},
		Refresh:      kafkaClusterProvisionStatus(c.cmkApiContext(ctx), c, environmentId, clusterId),
		Timeout:      getTimeoutFor(clusterType),
		Delay:        5 * time.Second,
		PollInterval: 1 * time.Minute,
	}

	log.Printf("[DEBUG] Waiting for Kafka cluster provisioning to become %s", stateDone)
	_, err := stateConf.WaitForStateContext(c.cmkApiContext(ctx))
	return err
}

func waitForKafkaClusterCkuUpdateToComplete(ctx context.Context, c *Client, environmentId, clusterId string, cku int32) error {
	stateConf := &resource.StateChangeConf{
		Pending:      []string{stateInProgress},
		Target:       []string{stateDone},
		Refresh:      kafkaClusterCkuUpdateStatus(c.cmkApiContext(ctx), c, environmentId, clusterId, cku),
		Timeout:      24 * time.Hour,
		Delay:        5 * time.Second,
		PollInterval: 1 * time.Minute,
	}

	log.Printf("[DEBUG] Waiting for Kafka cluster provisioning to become %s", stateDone)
	_, err := stateConf.WaitForStateContext(c.cmkApiContext(ctx))
	return err
}

func waitForKafkaTopicToBeDeleted(ctx context.Context, c *KafkaRestClient, topicName string) error {
	stateConf := &resource.StateChangeConf{
		Pending:      []string{stateInProgress},
		Target:       []string{stateDone},
		Refresh:      kafkaTopicStatus(c.apiContext(ctx), c, topicName),
		Timeout:      1 * time.Hour,
		Delay:        10 * time.Second,
		PollInterval: 1 * time.Minute,
	}

	log.Printf("[DEBUG] Waiting for Kafka topic to be deleted")
	_, err := stateConf.WaitForStateContext(c.apiContext(ctx))
	return err
}

func kafkaTopicStatus(ctx context.Context, c *KafkaRestClient, topicName string) resource.StateRefreshFunc {
	return func() (result interface{}, s string, err error) {
		kafkaTopic, resp, err := c.apiClient.TopicV3Api.GetKafkaV3Topic(c.apiContext(ctx), c.clusterId, topicName)
		if err != nil {
			log.Printf("[WARN] Kafka topic get failed for id %s, %v, %s", topicName, resp, err)

			// 404 means that the topic has been deleted
			isResourceNotFound := HasStatusNotFound(resp)
			if isResourceNotFound {
				// Result (the 1st argument) can't be nil
				return 0, stateDone, nil
			}
		}
		return kafkaTopic, stateInProgress, nil
	}
}

func kafkaClusterCkuUpdateStatus(ctx context.Context, c *Client, environmentId string, clusterId string, desiredCku int32) resource.StateRefreshFunc {
	return func() (result interface{}, s string, err error) {
		cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, environmentId, clusterId)
		if err != nil {
			log.Printf("[ERROR] Failed to fetch kafka cluster (%s): %+v, %s", clusterId, resp, err)
			return nil, stateUnknown, err
		}

		jsonCluster, _ := cluster.MarshalJSON()
		log.Printf("[DEBUG] Kafka cluster %s", jsonCluster)

		log.Printf("[DEBUG] Waiting for CKU update of Kafka cluster")
		// Wail until actual # of CKUs is the same as desired one
		// spec.cku is the user’s desired # of CKUs, and status.cku is the current # of CKUs in effect
		// because the change is still pending, for example
		// Use desiredCku on the off chance that API will not work as expected (i.e., spec.cku = status.cku during expansion).
		// CAPAC-293
		if cluster.Status.GetCku() == cluster.Spec.Config.CmkV2Dedicated.Cku && cluster.Status.GetCku() == desiredCku {
			return cluster, stateDone, nil
		}
		return cluster, stateInProgress, nil
	}
}

func kafkaClusterProvisionStatus(ctx context.Context, c *Client, environmentId string, clusterId string) resource.StateRefreshFunc {
	return func() (result interface{}, s string, err error) {
		cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, environmentId, clusterId)
		if err != nil {
			log.Printf("[ERROR] Kafka cluster get failed for id %s, %+v, %s", clusterId, resp, err)
			return nil, stateUnknown, err
		}

		jsonCluster, _ := cluster.MarshalJSON()
		log.Printf("[DEBUG] Kafka cluster %s", jsonCluster)

		if strings.ToUpper(c.waitUntil) == waitUntilProvisioned {
			log.Printf("[DEBUG] Waiting for Kafka cluster to be PROVISIONED: current status %s", cluster.Status.GetPhase())
			if cluster.Status.GetPhase() == waitUntilProvisioned {
				return cluster, stateDone, nil
			} else if cluster.Status.GetPhase() == stateFailed {
				return nil, stateFailed, fmt.Errorf("[ERROR] Kafka cluster provisioning has failed")
			}
			return cluster, stateInProgress, nil
		} else if strings.ToUpper(c.waitUntil) == waitUntilBootstrapAvailable {
			log.Printf("[DEBUG] Waiting for Kafka cluster's boostrap endpoint to be available")
			if cluster.Spec.GetKafkaBootstrapEndpoint() == "" {
				return cluster, stateInProgress, nil
			}
			return cluster, stateDone, nil
		}

		return cluster, stateDone, nil
	}
}
