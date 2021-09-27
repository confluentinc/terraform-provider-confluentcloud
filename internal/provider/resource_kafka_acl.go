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
	"strings"
)

const (
	paramResourceName = "resource_name"
	paramResourceType = "resource_type"
	paramPatternType  = "pattern_type"
	paramPrincipal    = "principal"
	paramHost         = "host"
	paramOperation    = "operation"
	paramPermission   = "permission"
)

var acceptedResourceTypes = []string{"UNKNOWN", "ANY", "TOPIC", "GROUP", "CLUSTER", "TRANSACTIONAL_ID", "DELEGATION_TOKEN"}
var acceptedPatternTypes = []string{"UNKNOWN", "ANY", "MATCH", "LITERAL", "PREFIXED"}
var acceptedOperations = []string{"UNKNOWN", "ANY", "ALL", "READ", "WRITE", "CREATE", "DELETE", "ALTER", "DESCRIBE", "CLUSTER_ACTION", "DESCRIBE_CONFIGS", "ALTER_CONFIGS", "IDEMPOTENT_WRITE"}
var acceptedPermissions = []string{"UNKNOWN", "ANY", "DENY", "ALLOW"}

func extractResourceName(d *schema.ResourceData) string {
	resourceName := d.Get(paramResourceName).(string)
	return resourceName
}
func extractResourceType(d *schema.ResourceData) string {
	resourceType := d.Get(paramResourceType).(string)
	return resourceType
}
func extractPatternType(d *schema.ResourceData) string {
	patternType := d.Get(paramPatternType).(string)
	return patternType
}
func extractPrincipal(d *schema.ResourceData) string {
	principal := d.Get(paramPrincipal).(string)
	return principal
}
func extractHost(d *schema.ResourceData) string {
	host := d.Get(paramHost).(string)
	return host
}
func extractOperation(d *schema.ResourceData) string {
	operation := d.Get(paramOperation).(string)
	return operation
}
func extractPermission(d *schema.ResourceData) string {
	permission := d.Get(paramPermission).(string)
	return permission
}

func extractAcl(d *schema.ResourceData) (Acl, error) {
	resourceType, err := stringToAclResourceType(extractResourceType(d))
	if err != nil {
		return Acl{}, err
	}
	patternType, err := stringToAclPatternType(extractPatternType(d))
	if err != nil {
		return Acl{}, err
	}
	operation, err := stringToAclOperation(extractOperation(d))
	if err != nil {
		return Acl{}, err
	}
	permission, err := stringToAclPermission(extractPermission(d))
	if err != nil {
		return Acl{}, err
	}
	return Acl{
		ResourceType: resourceType,
		ResourceName: extractResourceName(d),
		PatternType:  patternType,
		Principal:    extractPrincipal(d),
		Host:         extractHost(d),
		Operation:    operation,
		Permission:   permission,
	}, nil
}

func kafkaAclResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: kafkaAclCreate,
		ReadContext:   kafkaAclRead,
		DeleteContext: kafkaAclDelete,
		Schema: map[string]*schema.Schema{
			paramClusterId: clusterIdSchema(),
			paramResourceType: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The type of the resource.",
				ValidateFunc: validation.StringInSlice(acceptedResourceTypes, false),
			},
			paramResourceName: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The resource name for the ACL.",
			},
			paramPatternType: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The pattern type for the ACL.",
				ValidateFunc: validation.StringInSlice(acceptedPatternTypes, false),
			},
			paramPrincipal: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The principal for the ACL.",
			},
			paramHost: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The host for the ACL.",
			},
			paramOperation: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The operation type for the ACL.",
				ValidateFunc: validation.StringInSlice(acceptedOperations, false),
			},
			paramPermission: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The permission for the ACL.",
				ValidateFunc: validation.StringInSlice(acceptedPermissions, false),
			},
			paramHttpEndpoint: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The REST endpoint of the Kafka cluster (e.g., `https://pkc-00000.us-central1.gcp.confluent.cloud:443`).",
			},
			paramCredentials: credentialsSchema(),
		},
	}
}

func kafkaAclCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)
	updateKafkaRestClient(c, d)

	clusterId := extractClusterId(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	acl, err := extractAcl(d)
	if err != nil {
		return diag.FromErr(err)
	}
	kafkaAclRequestData := kafkarestv3.CreateAclRequestData{
		ResourceType: acl.ResourceType,
		ResourceName: acl.ResourceName,
		PatternType:  acl.PatternType,
		Principal:    acl.Principal,
		Host:         acl.Host,
		Operation:    acl.Operation,
		Permission:   acl.Permission,
	}

	resp, err := executeKafkaAclCreate(c.kafkaRestApiContext(ctx, clusterApiKey, clusterApiSecret), c, clusterId, kafkaAclRequestData)

	if err != nil {
		log.Printf("[ERROR] Kafka ACL create failed %v, %v, %s", kafkaAclRequestData, resp, err)
		return diag.FromErr(err)
	}
	kafkaAclId := createKafkaAclId(clusterId, acl)
	d.SetId(kafkaAclId)
	log.Printf("[DEBUG] Created kafka ACL %s", kafkaAclId)
	return nil
}

func executeKafkaAclCreate(ctx context.Context, c *Client, clusterId string, requestData kafkarestv3.CreateAclRequestData) (*http.Response, error) {
	opts := &kafkarestv3.CreateKafkaV3AclsOpts{
		CreateAclRequestData: optional.NewInterface(requestData),
	}
	return c.kafkaRestClient.ACLV3Api.CreateKafkaV3Acls(ctx, clusterId, opts)
}

func kafkaAclDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)
	updateKafkaRestClient(c, d)

	clusterId := extractClusterId(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	acl, err := extractAcl(d)
	if err != nil {
		return diag.FromErr(err)
	}
	opts := &kafkarestv3.DeleteKafkaV3AclsOpts{
		ResourceType: optional.NewInterface(acl.ResourceType),
		ResourceName: optional.NewString(acl.ResourceName),
		PatternType:  optional.NewInterface(acl.PatternType),
		Principal:    optional.NewString(acl.Principal),
		Host:         optional.NewString(acl.Host),
		Operation:    optional.NewInterface(acl.Operation),
		Permission:   optional.NewInterface(acl.Permission),
	}

	_, _, err = c.kafkaRestClient.ACLV3Api.DeleteKafkaV3Acls(c.kafkaRestApiContext(ctx, clusterApiKey, clusterApiSecret), clusterId, opts)

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting kafka ACL (%s), err: %s", d.Id(), err))
	}

	return nil
}

func executeKafkaAclRead(ctx context.Context, c *Client, clusterId string, opts *kafkarestv3.GetKafkaV3AclsOpts) (kafkarestv3.AclDataList, *http.Response, error) {
	return c.kafkaRestClient.ACLV3Api.GetKafkaV3Acls(ctx, clusterId, opts)
}

func kafkaAclRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka ACL read for %s", d.Id())

	c := meta.(*Client)
	updateKafkaRestClient(c, d)

	clusterId := extractClusterId(d)
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	acl, err := extractAcl(d)
	if err != nil {
		return diag.FromErr(err)
	}
	opts := &kafkarestv3.GetKafkaV3AclsOpts{
		ResourceType: optional.NewInterface(acl.ResourceType),
		ResourceName: optional.NewString(acl.ResourceName),
		PatternType:  optional.NewInterface(acl.PatternType),
		Principal:    optional.NewString(acl.Principal),
		Host:         optional.NewString(acl.Host),
		Operation:    optional.NewInterface(acl.Operation),
		Permission:   optional.NewInterface(acl.Permission),
	}

	_, resp, err := executeKafkaAclRead(c.kafkaRestApiContext(ctx, clusterApiKey, clusterApiSecret), c, clusterId, opts)
	if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
		// https://learn.hashicorp.com/tutorials/terraform/provider-setup?in=terraform/providers
		// If the resource isn't available, set the ID to an empty string so Terraform "destroys" the resource in state.
		d.SetId("")
		return nil
	}
	if err != nil {
		log.Printf("[ERROR] Kafka ACL get failed for id %s, %v, %s", d.Id(), resp, err)
	}
	d.SetId(createKafkaAclId(clusterId, acl))
	return nil
}

func createKafkaAclId(clusterId string, acl Acl) string {
	return strings.Join([]string{
		clusterId,
		string(acl.ResourceType),
		acl.ResourceName,
		string(acl.PatternType),
		acl.Principal,
		acl.Host,
		string(acl.Operation),
		string(acl.Permission),
	}, "/")
}
