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
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/walkerus/go-wiremock"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

const (
	scenarioStateTopicHasBeenCreated = "A new topic has been just created"
	scenarioStateTopicHasBeenDeleted = "The topic has been deleted"
	topicScenarioName                = "confluentcloud_kafka_topic Resource Lifecycle"
	clusterId                        = "lkc-190073"
	replicationFactor                = 3
	partitionCount                   = 4
	firstConfigName                  = "max.message.bytes"
	firstConfigValue                 = "12345"
	secondConfigName                 = "retention.ms"
	secondConfigValue                = "6789"
	topicName                        = "test_topic_name"
	topicResourceLabel               = "test_topic_resource_label"
	clusterApiKey                    = "foo"
	clusterApiSecret                 = "bar"
)

var fullTopicResourceLabel = fmt.Sprintf("confluentcloud_kafka_topic.%s", topicResourceLabel)
var createKafkaTopicPath = fmt.Sprintf("/kafka/v3/clusters/%s/topics", clusterId)
var readKafkaTopicPath = fmt.Sprintf("/kafka/v3/clusters/%s/topics/%s", clusterId, topicName)
var readKafkaTopicConfigPath = fmt.Sprintf("/kafka/v3/clusters/%s/topics/%s/configs", clusterId, topicName)

func TestAccTopic(t *testing.T) {
	containerPort := "8080"
	containerPortTcp := fmt.Sprintf("%s/tcp", containerPort)
	ctx := context.Background()
	listeningPort := wait.ForListeningPort(nat.Port(containerPortTcp))
	req := testcontainers.ContainerRequest{
		Image:        "rodolpheche/wiremock",
		ExposedPorts: []string{containerPortTcp},
		WaitingFor:   listeningPort,
	}
	wiremockContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	require.NoError(t, err)

	// nolint:errcheck
	defer wiremockContainer.Terminate(ctx)

	host, err := wiremockContainer.Host(ctx)
	require.NoError(t, err)

	wiremockHttpMappedPort, err := wiremockContainer.MappedPort(ctx, nat.Port(containerPort))
	require.NoError(t, err)

	mockServerUrl := fmt.Sprintf("http://%s:%s", host, wiremockHttpMappedPort.Port())
	confluentCloudBaseUrl := ""
	wiremockClient := wiremock.NewClient(mockServerUrl)
	// nolint:errcheck
	defer wiremockClient.Reset()

	// nolint:errcheck
	defer wiremockClient.ResetAllScenarios()
	createTopicResponse, _ := ioutil.ReadFile("../testdata/kafka_topic/create_kafka_topic.json")
	createTopicStub := wiremock.Post(wiremock.URLPathEqualTo(createKafkaTopicPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(wiremock.ScenarioStateStarted).
		WillSetStateTo(scenarioStateTopicHasBeenCreated).
		WillReturn(
			string(createTopicResponse),
			contentTypeJSONHeader,
			http.StatusCreated,
		)
	_ = wiremockClient.StubFor(createTopicStub)

	readCreatedTopicResponse, _ := ioutil.ReadFile("../testdata/kafka_topic/read_created_kafka_topic.json")
	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo(readKafkaTopicPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(scenarioStateTopicHasBeenCreated).
		WillReturn(
			string(readCreatedTopicResponse),
			contentTypeJSONHeader,
			http.StatusOK,
		))

	readCreatedTopicConfigResponse, _ := ioutil.ReadFile("../testdata/kafka_topic/read_created_kafka_topic_config.json")
	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo(readKafkaTopicConfigPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(wiremock.ScenarioStateStarted).
		WillReturn(
			string(readCreatedTopicConfigResponse),
			contentTypeJSONHeader,
			http.StatusOK,
		))
	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo(readKafkaTopicConfigPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(scenarioStateTopicHasBeenCreated).
		WillReturn(
			string(readCreatedTopicConfigResponse),
			contentTypeJSONHeader,
			http.StatusOK,
		))

	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo(readKafkaTopicPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(scenarioStateTopicHasBeenDeleted).
		WillReturn(
			"",
			contentTypeJSONHeader,
			http.StatusNotFound,
		))

	deleteTopicStub := wiremock.Delete(wiremock.URLPathEqualTo(readKafkaTopicPath)).
		InScenario(topicScenarioName).
		WhenScenarioStateIs(scenarioStateTopicHasBeenCreated).
		WillSetStateTo(scenarioStateTopicHasBeenDeleted).
		WillReturn(
			"",
			contentTypeJSONHeader,
			http.StatusNoContent,
		)
	_ = wiremockClient.StubFor(deleteTopicStub)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccCheckTopicDestroy,
		// https://www.terraform.io/docs/extend/testing/acceptance-tests/teststep.html
		// https://www.terraform.io/docs/extend/best-practices/testing.html#built-in-patterns
		Steps: []resource.TestStep{
			{
				Config: testAccCheckTopicConfig(confluentCloudBaseUrl, mockServerUrl),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTopicExists(fullTopicResourceLabel),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "kafka", clusterId),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "id", fmt.Sprintf("%s/%s", clusterId, topicName)),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "topic_name", topicName),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "partitions_count", strconv.Itoa(partitionCount)),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "replication_factor", strconv.Itoa(replicationFactor)),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "http_endpoint", mockServerUrl),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "config.%", "2"),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "config.max.message.bytes", "12345"),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "config.retention.ms", "6789"),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "credentials.#", "1"),
					resource.TestCheckResourceAttr(fullTopicResourceLabel, "credentials.0.%", "2"),
				),
			},
		},
	})

	checkStubCount(t, wiremockClient, createTopicStub, fmt.Sprintf("POST %s", createKafkaTopicPath), expectedCountOne)
	checkStubCount(t, wiremockClient, deleteTopicStub, fmt.Sprintf("DELETE %s", readKafkaTopicPath), expectedCountOne)
}

func testAccCheckTopicDestroy(s *terraform.State) error {
	c := testAccProvider.Meta().(*Client)
	// Loop through the resources in state, verifying each Kafka topic is destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "confluentcloud_kafka_topic" {
			continue
		}
		deletedTopicId := rs.Primary.ID
		_, response, err := c.kafkaRestClient.TopicV3Api.GetKafkaV3Topic(c.kafkaRestApiContext(context.Background(), clusterApiKey, clusterApiSecret), clusterId, topicName)
		if response != nil && (response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound) {
			return nil
		} else if err == nil && deletedTopicId != "" {
			// Otherwise return the error
			if deletedTopicId == rs.Primary.ID {
				return fmt.Errorf("topic (%s) still exists", rs.Primary.ID)
			}
		}
		return err
	}
	return nil
}

func testAccCheckTopicConfig(confluentCloudBaseUrl, mockServerUrl string) string {
	return fmt.Sprintf(`
	provider "confluentcloud" {
      endpoint = "%s"
    }
	resource "confluentcloud_kafka_topic" "%s" {
	  kafka = "%s"
	
	  topic_name = "%s"
	  partitions_count = "%d"
	  replication_factor = "%d"
	  http_endpoint = "%s"
	
	  config = {
		"%s" = "%s"
		"%s" = "%s"
	  }

	  credentials {
		key = "test_key"
		secret = "test_secret"
	  }
	}
	`, confluentCloudBaseUrl, topicResourceLabel, clusterId, topicName, partitionCount, replicationFactor, mockServerUrl, firstConfigName, firstConfigValue, secondConfigName, secondConfigValue)
}

func testAccCheckTopicExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]

		if !ok {
			return fmt.Errorf("%s topic has not been found", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("ID has not been set for %s topic", n)
		}

		return nil
	}
}
