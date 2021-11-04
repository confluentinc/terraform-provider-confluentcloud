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
		Importer: &schema.ResourceImporter{
			StateContext: kafkaAclImport,
		},
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
				Optional:    true,
				Default:     "*",
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
	httpEndpoint := extractHttpEndpoint(d)
	updateKafkaRestClient(c, httpEndpoint)

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
	httpEndpoint := extractHttpEndpoint(d)
	updateKafkaRestClient(c, httpEndpoint)

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

	clusterId := extractClusterId(d)
	acl, err := extractAcl(d)
	if err != nil {
		return diag.FromErr(err)
	}
	clusterApiKey, clusterApiSecret, err := extractClusterApiKeyAndApiSecret(d)
	if err != nil {
		return diag.FromErr(err)
	}
	httpEndpoint := extractHttpEndpoint(d)

	_, err = readAndSetAclResourceConfigurationArguments(ctx, d, meta, clusterId, acl, clusterApiKey, clusterApiSecret, httpEndpoint)

	return diag.FromErr(err)
}

func createKafkaAclId(clusterId string, acl Acl) string {
	return fmt.Sprintf("%s/%s", clusterId, strings.Join([]string{
		string(acl.ResourceType),
		acl.ResourceName,
		string(acl.PatternType),
		acl.Principal,
		acl.Host,
		string(acl.Operation),
		string(acl.Permission),
	}, "#"))
}

func readAndSetAclResourceConfigurationArguments(ctx context.Context, d *schema.ResourceData, meta interface{}, clusterId string, acl Acl, kafkaApiKey, kafkaApiSecret, httpEndpoint string) ([]*schema.ResourceData, error) {
	c := meta.(*Client)
	updateKafkaRestClient(c, httpEndpoint)

	ctx = c.kafkaRestApiContext(ctx, kafkaApiKey, kafkaApiSecret)

	opts := &kafkarestv3.GetKafkaV3AclsOpts{
		ResourceType: optional.NewInterface(acl.ResourceType),
		ResourceName: optional.NewString(acl.ResourceName),
		PatternType:  optional.NewInterface(acl.PatternType),
		Principal:    optional.NewString(acl.Principal),
		Host:         optional.NewString(acl.Host),
		Operation:    optional.NewInterface(acl.Operation),
		Permission:   optional.NewInterface(acl.Permission),
	}

	_, resp, err := executeKafkaAclRead(ctx, c, clusterId, opts)
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		// https://learn.hashicorp.com/tutorials/terraform/provider-setup?in=terraform/providers
		// If the resource isn't available, set the ID to an empty string so Terraform "destroys" the resource in state.
		d.SetId("")
		return nil, nil
	}
	if err != nil {
		log.Printf("[ERROR] Kafka ACL get failed for id %s, %v, %s", acl, resp, err)
	}
	if err == nil {
		err = d.Set(paramClusterId, clusterId)
	}
	if err == nil {
		err = d.Set(paramResourceType, acl.ResourceType)
	}
	if err == nil {
		err = d.Set(paramResourceName, acl.ResourceName)
	}
	if err == nil {
		err = d.Set(paramPatternType, acl.PatternType)
	}
	if err == nil {
		err = d.Set(paramPrincipal, acl.Principal)
	}
	if err == nil {
		err = d.Set(paramHost, acl.Host)
	}
	if err == nil {
		err = d.Set(paramOperation, acl.Operation)
	}
	if err == nil {
		err = d.Set(paramPermission, acl.Permission)
	}
	if err == nil {
		err = setKafkaCredentials(kafkaApiKey, kafkaApiSecret, d)
	}
	if err == nil {
		err = d.Set(paramHttpEndpoint, httpEndpoint)
	}
	d.SetId(createKafkaAclId(clusterId, acl))
	return []*schema.ResourceData{d}, err
}

func kafkaAclImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	log.Printf("[INFO] Kafka ACL import for %s", d.Id())

	kafkaImportEnvVars, err := checkEnvironmentVariablesForKafkaImportAreSet()
	if err != nil {
		return nil, err
	}

	clusterIdAndSerializedAcl := d.Id()

	parts := strings.Split(clusterIdAndSerializedAcl, "/")

	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for kafka ACL import: expected '<lkc ID>/<resource type>#<resource name>#<pattern type>#<principal>#<host>#<operation>#<permission>'")
	}

	clusterId := parts[0]
	serializedAcl := parts[1]

	acl, err := deserializeAcl(serializedAcl)
	if err != nil {
		return nil, err
	}

	return readAndSetAclResourceConfigurationArguments(ctx, d, meta, clusterId, acl, kafkaImportEnvVars.kafkaApiKey, kafkaImportEnvVars.kafkaApiSecret, kafkaImportEnvVars.kafkaHttpEndpoint)
}

func deserializeAcl(serializedAcl string) (Acl, error) {
	parts := strings.Split(serializedAcl, "#")
	if len(parts) != 7 {
		return Acl{}, fmt.Errorf("invalid format for kafka ACL import: expected '<lkc ID>/<resource type>#<resource name>#<pattern type>#<principal>#<host>#<operation>#<permission>'")
	}

	resourceType, err := stringToAclResourceType(parts[0])
	if err != nil {
		return Acl{}, err
	}
	patternType, err := stringToAclPatternType(parts[2])
	if err != nil {
		return Acl{}, err
	}
	operation, err := stringToAclOperation(parts[5])
	if err != nil {
		return Acl{}, err
	}
	permission, err := stringToAclPermission(parts[6])
	if err != nil {
		return Acl{}, err
	}

	return Acl{
		ResourceType: resourceType,
		ResourceName: parts[1],
		PatternType:  patternType,
		Principal:    parts[3],
		Host:         parts[4],
		Operation:    operation,
		Permission:   permission,
	}, nil
}
