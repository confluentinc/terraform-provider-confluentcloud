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
	cmk "github.com/confluentinc/ccloud-sdk-go-v2/cmk/v2"
	iamv1 "github.com/confluentinc/ccloud-sdk-go-v2/iam/v1"
	iam "github.com/confluentinc/ccloud-sdk-go-v2/iam/v2"
	mds "github.com/confluentinc/ccloud-sdk-go-v2/mds/v2"
	org "github.com/confluentinc/ccloud-sdk-go-v2/org/v2"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
	"strings"
)

const (
	terraformProviderUserAgent = "terraform-provider-confluentcloud"
)

const (
	paramCloud       = "cloud"
	paramRegion      = "region"
	paramEnvironment = "environment"
	paramWaitUntil   = "wait_until"
	paramId          = "id"
	paramDisplayName = "display_name"
	paramDescription = "description"
)

type Client struct {
	iamClient              *iam.APIClient
	iamV1Client            *iamv1.APIClient
	cmkClient              *cmk.APIClient
	orgClient              *org.APIClient
	kafkaRestClientFactory *KafkaRestClientFactory
	mdsClient              *mds.APIClient
	userAgent              string
	apiKey                 string
	apiSecret              string
	waitUntil              string
}

// Customize configs for terraform-plugin-docs
func init() {
	schema.DescriptionKind = schema.StringMarkdown

	schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
		descriptionWithDefault := s.Description
		if s.Default != nil {
			descriptionWithDefault += fmt.Sprintf(" Defaults to `%v`.", s.Default)
		}
		return strings.TrimSpace(descriptionWithDefault)
	}
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		log.Printf("[INFO] Creating Confluent Cloud Provider")
		provider := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"api_key": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("CONFLUENT_CLOUD_API_KEY", ""),
					Description: "The Confluent Cloud API Key.",
				},
				"api_secret": {
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("CONFLUENT_CLOUD_API_SECRET", ""),
					Description: "The Confluent Cloud API Secret.",
				},
				"endpoint": {
					Type:        schema.TypeString,
					Optional:    true,
					Default:     "https://api.confluent.cloud",
					Description: "The base endpoint of Confluent Cloud API.",
				},
				paramWaitUntil: {
					Type:        schema.TypeString,
					Optional:    true,
					Default:     waitUntilProvisioned,
					Description: "Terraform apply will wait until the specified field that is populated.",
					ValidateDiagFunc: func(i interface{}, path cty.Path) diag.Diagnostics {
						waitUntil := i.(string)
						if waitUntil != waitUntilBootstrapAvailable && waitUntil != waitUntilProvisioned && waitUntil != waitUntilNone {
							return diag.Errorf("wait until can only be one of %s, %s or %s", waitUntilProvisioned, waitUntilBootstrapAvailable, waitUntilNone)
						}
						return nil
					},
				},
			},
			DataSourcesMap: map[string]*schema.Resource{
				"confluentcloud_kafka_cluster":   kafkaDataSource(),
				"confluentcloud_kafka_topic":     kafkaTopicDataSource(),
				"confluentcloud_environment":     environmentDataSource(),
				"confluentcloud_service_account": serviceAccountDataSource(),
			},
			ResourcesMap: map[string]*schema.Resource{
				"confluentcloud_kafka_cluster":   kafkaResource(),
				"confluentcloud_environment":     environmentResource(),
				"confluentcloud_service_account": serviceAccountResource(),
				"confluentcloud_kafka_topic":     kafkaTopicResource(),
				"confluentcloud_kafka_acl":       kafkaAclResource(),
				"confluentcloud_role_binding":    roleBindingResource(),
			},
		}

		provider.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
			return providerConfigure(ctx, d, provider, version)
		}

		return provider
	}
}

// https://github.com/hashicorp/terraform-plugin-sdk/issues/155#issuecomment-489699737
////  alternative - https://github.com/hashicorp/terraform-plugin-sdk/issues/248#issuecomment-725013327
func environmentSchema() *schema.Schema {
	return &schema.Schema{
		Type: schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramId: {
					Type:        schema.TypeString,
					Required:    true,
					ForceNew:    true,
					Description: "The unique identifier for the environment.",
				},
			},
		},
		Required:    true,
		MaxItems:    1,
		ForceNew:    true,
		Description: "Environment objects represent an isolated namespace for your Confluent resources for organizational purposes.",
	}
}

// https://github.com/hashicorp/terraform-plugin-sdk/issues/155#issuecomment-489699737
////  alternative - https://github.com/hashicorp/terraform-plugin-sdk/issues/248#issuecomment-725013327
func environmentDataSourceSchema() *schema.Schema {
	return &schema.Schema{
		Type: schema.TypeList,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramId: {
					Type:     schema.TypeString,
					Required: true,
				},
			},
		},
		Required: true,
		MaxItems: 1,
	}
}

func validEnvironmentId(d *schema.ResourceData) (string, error) {
	envIdResource := extractEnvironmentId(d)
	if envIdResource != nil {
		return *envIdResource, nil
	}
	return "", fmt.Errorf("the environment ID must be provided")
}

func extractEnvironmentId(d *schema.ResourceData) *string {
	envData := d.Get(paramEnvironment).([]interface{})
	if len(envData) == 0 || envData[0] == nil {
		return nil
	}
	envMap := envData[0].(map[string]interface{})
	if environmentId, ok := envMap[paramId].(string); ok && len(environmentId) > 0 {
		return &environmentId
	}
	return nil
}

func setEnvironmentId(environmentId string, d *schema.ResourceData) error {
	return d.Set(paramEnvironment, []interface{}{map[string]interface{}{
		paramId: environmentId,
	}})
}

func providerConfigure(ctx context.Context, d *schema.ResourceData, p *schema.Provider, providerVersion string) (interface{}, diag.Diagnostics) {
	log.Printf("[INFO] Initializing ConfluentCloud provider")
	endpoint := d.Get("endpoint").(string)
	apiKey := d.Get("api_key").(string)
	apiSecret := d.Get("api_secret").(string)
	waitUntil := d.Get(paramWaitUntil).(string)

	userAgent := p.UserAgent(terraformProviderUserAgent, fmt.Sprintf("%s (https://confluent.cloud; support@confluent.io)", providerVersion))

	cmkCfg := cmk.NewConfiguration()
	iamCfg := iam.NewConfiguration()
	iamV1Cfg := iamv1.NewConfiguration()
	mdsCfg := mds.NewConfiguration()
	orgCfg := org.NewConfiguration()

	cmkCfg.Servers[0].URL = endpoint
	iamCfg.Servers[0].URL = endpoint
	iamV1Cfg.Servers[0].URL = endpoint
	mdsCfg.Servers[0].URL = endpoint
	orgCfg.Servers[0].URL = endpoint

	cmkCfg.UserAgent = userAgent
	iamCfg.UserAgent = userAgent
	iamV1Cfg.UserAgent = userAgent
	mdsCfg.UserAgent = userAgent
	orgCfg.UserAgent = userAgent

	cmkCfg.HTTPClient = createRetryableHttpClientWithExponentialBackoff()
	iamCfg.HTTPClient = createRetryableHttpClientWithExponentialBackoff()
	iamV1Cfg.HTTPClient = createRetryableHttpClientWithExponentialBackoff()
	mdsCfg.HTTPClient = createRetryableHttpClientWithExponentialBackoff()
	orgCfg.HTTPClient = createRetryableHttpClientWithExponentialBackoff()

	client := Client{
		cmkClient:              cmk.NewAPIClient(cmkCfg),
		iamClient:              iam.NewAPIClient(iamCfg),
		iamV1Client:            iamv1.NewAPIClient(iamV1Cfg),
		orgClient:              org.NewAPIClient(orgCfg),
		kafkaRestClientFactory: &KafkaRestClientFactory{userAgent: userAgent},
		mdsClient:              mds.NewAPIClient(mdsCfg),
		userAgent:              userAgent,
		apiKey:                 apiKey,
		apiSecret:              apiSecret,
		waitUntil:              waitUntil,
	}

	return &client, nil
}
