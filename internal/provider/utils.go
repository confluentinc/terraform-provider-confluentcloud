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
	kafkarestv3 "github.com/confluentinc/ccloud-sdk-go-v2/kafkarest/v3"
	mds "github.com/confluentinc/ccloud-sdk-go-v2/mds/v2"
	org "github.com/confluentinc/ccloud-sdk-go-v2/org/v2"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const crnKafkaSuffix = "/kafka="

func (c *Client) cmkApiContext(ctx context.Context) context.Context {
	if c.apiKey != "" && c.apiSecret != "" {
		return context.WithValue(context.Background(), cmk.ContextBasicAuth, cmk.BasicAuth{
			UserName: c.apiKey,
			Password: c.apiSecret,
		})
	}
	log.Printf("[WARN] Could not find credentials for Confluent Cloud")
	return ctx
}

func (c *Client) iamApiContext(ctx context.Context) context.Context {
	if c.apiKey != "" && c.apiSecret != "" {
		return context.WithValue(context.Background(), iam.ContextBasicAuth, iam.BasicAuth{
			UserName: c.apiKey,
			Password: c.apiSecret,
		})
	}
	log.Printf("[WARN] Could not find credentials for Confluent Cloud")
	return ctx
}

func (c *Client) iamV1ApiContext(ctx context.Context) context.Context {
	if c.apiKey != "" && c.apiSecret != "" {
		return context.WithValue(context.Background(), iamv1.ContextBasicAuth, iamv1.BasicAuth{
			UserName: c.apiKey,
			Password: c.apiSecret,
		})
	}
	log.Printf("[WARN] Could not find credentials for Confluent Cloud")
	return ctx
}

func (c *Client) mdsApiContext(ctx context.Context) context.Context {
	if c.apiKey != "" && c.apiSecret != "" {
		return context.WithValue(context.Background(), mds.ContextBasicAuth, mds.BasicAuth{
			UserName: c.apiKey,
			Password: c.apiSecret,
		})
	}
	log.Printf("[WARN] Could not find credentials for Confluent Cloud")
	return ctx
}

func (c *Client) orgApiContext(ctx context.Context) context.Context {
	if c.apiKey != "" && c.apiSecret != "" {
		return context.WithValue(context.Background(), org.ContextBasicAuth, org.BasicAuth{
			UserName: c.apiKey,
			Password: c.apiSecret,
		})
	}
	log.Printf("[WARN] Could not find credentials for Confluent Cloud")
	return ctx
}

func getTimeoutFor(clusterType string) time.Duration {
	if clusterType == kafkaClusterTypeDedicated {
		return 24 * time.Hour
	} else {
		return 1 * time.Hour
	}
}

func stringToAclResourceType(aclResourceType string) (kafkarestv3.AclResourceType, error) {
	switch aclResourceType {
	case "UNKNOWN":
		return kafkarestv3.ACLRESOURCETYPE_UNKNOWN, nil
	case "ANY":
		return kafkarestv3.ACLRESOURCETYPE_ANY, nil
	case "TOPIC":
		return kafkarestv3.ACLRESOURCETYPE_TOPIC, nil
	case "GROUP":
		return kafkarestv3.ACLRESOURCETYPE_GROUP, nil
	case "CLUSTER":
		return kafkarestv3.ACLRESOURCETYPE_CLUSTER, nil
	case "TRANSACTIONAL_ID":
		return kafkarestv3.ACLRESOURCETYPE_TRANSACTIONAL_ID, nil
	case "DELEGATION_TOKEN":
		return kafkarestv3.ACLRESOURCETYPE_DELEGATION_TOKEN, nil
	}
	return "", fmt.Errorf("unknown ACL resource type was found: %s", aclResourceType)
}

func stringToAclPatternType(aclPatternType string) (kafkarestv3.AclPatternType, error) {
	switch aclPatternType {
	case "UNKNOWN":
		return kafkarestv3.ACLPATTERNTYPE_UNKNOWN, nil
	case "ANY":
		return kafkarestv3.ACLPATTERNTYPE_ANY, nil
	case "MATCH":
		return kafkarestv3.ACLPATTERNTYPE_MATCH, nil
	case "LITERAL":
		return kafkarestv3.ACLPATTERNTYPE_LITERAL, nil
	case "PREFIXED":
		return kafkarestv3.ACLPATTERNTYPE_PREFIXED, nil
	}
	return "", fmt.Errorf("unknown ACL pattern type was found: %s", aclPatternType)
}

func stringToAclOperation(aclOperation string) (kafkarestv3.AclOperation, error) {
	switch aclOperation {
	case "UNKNOWN":
		return kafkarestv3.ACLOPERATION_UNKNOWN, nil
	case "ANY":
		return kafkarestv3.ACLOPERATION_ANY, nil
	case "ALL":
		return kafkarestv3.ACLOPERATION_ALL, nil
	case "READ":
		return kafkarestv3.ACLOPERATION_READ, nil
	case "WRITE":
		return kafkarestv3.ACLOPERATION_WRITE, nil
	case "CREATE":
		return kafkarestv3.ACLOPERATION_CREATE, nil
	case "DELETE":
		return kafkarestv3.ACLOPERATION_DELETE, nil
	case "ALTER":
		return kafkarestv3.ACLOPERATION_ALTER, nil
	case "DESCRIBE":
		return kafkarestv3.ACLOPERATION_DESCRIBE, nil
	case "CLUSTER_ACTION":
		return kafkarestv3.ACLOPERATION_CLUSTER_ACTION, nil
	case "DESCRIBE_CONFIGS":
		return kafkarestv3.ACLOPERATION_DESCRIBE_CONFIGS, nil
	case "ALTER_CONFIGS":
		return kafkarestv3.ACLOPERATION_ALTER_CONFIGS, nil
	case "IDEMPOTENT_WRITE":
		return kafkarestv3.ACLOPERATION_IDEMPOTENT_WRITE, nil
	}
	return "", fmt.Errorf("unknown ACL operation was found: %s", aclOperation)
}

func stringToAclPermission(aclPermission string) (kafkarestv3.AclPermission, error) {
	switch aclPermission {
	case "UNKNOWN":
		return kafkarestv3.ACLPERMISSION_UNKNOWN, nil
	case "ANY":
		return kafkarestv3.ACLPERMISSION_ANY, nil
	case "DENY":
		return kafkarestv3.ACLPERMISSION_DENY, nil
	case "ALLOW":
		return kafkarestv3.ACLPERMISSION_ALLOW, nil
	}
	return "", fmt.Errorf("unknown ACL permission was found: %s", aclPermission)
}

type Acl struct {
	ResourceType kafkarestv3.AclResourceType
	ResourceName string
	PatternType  kafkarestv3.AclPatternType
	Principal    string
	Host         string
	Operation    kafkarestv3.AclOperation
	Permission   kafkarestv3.AclPermission
}

type KafkaImportEnvVars struct {
	kafkaApiKey       string
	kafkaApiSecret    string
	kafkaHttpEndpoint string
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func checkEnvironmentVariablesForKafkaImportAreSet() (KafkaImportEnvVars, error) {
	kafkaApiKey := getEnv("KAFKA_API_KEY", "")
	kafkaApiSecret := getEnv("KAFKA_API_SECRET", "")
	kafkaHttpEndpoint := getEnv("KAFKA_HTTP_ENDPOINT", "")
	canImport := kafkaApiKey != "" && kafkaApiSecret != "" && kafkaHttpEndpoint != ""
	if !canImport {
		return KafkaImportEnvVars{}, fmt.Errorf("KAFKA_API_KEY, KAFKA_API_SECRET, and KAFKA_HTTP_ENDPOINT must be set for kafka topic / ACL import")
	}
	return KafkaImportEnvVars{
		kafkaApiKey:       kafkaApiKey,
		kafkaApiSecret:    kafkaApiSecret,
		kafkaHttpEndpoint: kafkaHttpEndpoint,
	}, nil
}

type KafkaRestClient struct {
	apiClient        *kafkarestv3.APIClient
	clusterId        string
	clusterApiKey    string
	clusterApiSecret string
	httpEndpoint     string
}

func (c *KafkaRestClient) apiContext(ctx context.Context) context.Context {
	if c.clusterApiKey != "" && c.clusterApiSecret != "" {
		return context.WithValue(context.Background(), kafkarestv3.ContextBasicAuth, kafkarestv3.BasicAuth{
			UserName: c.clusterApiKey,
			Password: c.clusterApiSecret,
		})
	}
	log.Printf("[WARN] Could not find cluster credentials for Confluent Cloud for clusterId=%s", c.clusterId)
	return ctx
}

// Creates retryable HTTP client that performs automatic retries with exponential backoff for 429
// and 5** (except 501) errors. Otherwise, the response is returned and left to the caller to interpret.
func createRetryableHttpClientWithExponentialBackoff() *http.Client {
	retryClient := retryablehttp.NewClient()

	// Implicitly using default retry configuration
	// under the assumption is it's OK to spend retrying a single HTTP call around 15 seconds in total: 1 + 2 + 4 + 8
	// An exponential backoff equation: https://github.com/hashicorp/go-retryablehttp/blob/master/client.go#L493
	// retryWaitMax = math.Pow(2, float64(attemptNum)) * float64(retryWaitMin)
	// defaultRetryWaitMin = 1 * time.Second
	// defaultRetryWaitMax = 30 * time.Second
	// defaultRetryMax     = 4

	return retryClient.StandardClient()
}

type KafkaRestClientFactory struct {
	userAgent string
}

func (f KafkaRestClientFactory) CreateKafkaRestClient(httpEndpoint, clusterId, clusterApiKey, clusterApiSecret string) *KafkaRestClient {
	config := kafkarestv3.NewConfiguration()
	config.BasePath = httpEndpoint
	config.UserAgent = f.userAgent
	config.HTTPClient = createRetryableHttpClientWithExponentialBackoff()
	return &KafkaRestClient{
		apiClient:        kafkarestv3.NewAPIClient(config),
		clusterId:        clusterId,
		clusterApiKey:    clusterApiKey,
		clusterApiSecret: clusterApiSecret,
		httpEndpoint:     httpEndpoint,
	}
}

func extractStringAttributeFromListBlockOfSizeOne(d *schema.ResourceData, blockName, attributeName string) (string, error) {
	// d.Get() will return "" if the key is not present
	value := d.Get(fmt.Sprintf("%s.0.%s", blockName, attributeName)).(string)
	if value == "" {
		return "", fmt.Errorf("could not find %s attribute in %s block", attributeName, blockName)
	}
	return value, nil
}

// createDiagnosticsWithDetails will convert GenericOpenAPIError error into a Diagnostics with details.
// It should be used instead of diag.FromErr() in this project
// since diag.FromErr() returns just HTTP status code and its generic name (i.e., "400 Bad Request")
// (because of its usage of GenericOpenAPIError.Error()).
func createDiagnosticsWithDetails(err error) diag.Diagnostics {
	if err == nil {
		return nil
	}
	// At this point it's just status code and its generic name
	errorMessage := err.Error()

	// Add error.detail to the final error message
	if cmkError, ok := err.(cmk.GenericOpenAPIError); ok {
		if cmkFailure, ok := cmkError.Model().(cmk.Failure); ok {
			cmkFailureErrors := cmkFailure.GetErrors()
			if len(cmkFailureErrors) > 0 && cmkFailureErrors[0].Detail != nil {
				errorMessage = fmt.Sprintf("%s: %s", errorMessage, *cmkFailureErrors[0].Detail)
			}
		}
	}

	if iamError, ok := err.(iam.GenericOpenAPIError); ok {
		if iamFailure, ok := iamError.Model().(iam.Failure); ok {
			iamFailureErrors := iamFailure.GetErrors()
			if len(iamFailureErrors) > 0 && iamFailureErrors[0].Detail != nil {
				errorMessage = fmt.Sprintf("%s: %s", errorMessage, *iamFailureErrors[0].Detail)
			}
		}
	}

	if mdsError, ok := err.(mds.GenericOpenAPIError); ok {
		if mdsFailure, ok := mdsError.Model().(mds.Failure); ok {
			mdsFailureErrors := mdsFailure.GetErrors()
			if len(mdsFailureErrors) > 0 && mdsFailureErrors[0].Detail != nil {
				errorMessage = fmt.Sprintf("%s: %s", errorMessage, *mdsFailureErrors[0].Detail)
			}
		}
	}

	if orgError, ok := err.(org.GenericOpenAPIError); ok {
		if orgFailure, ok := orgError.Model().(org.Failure); ok {
			orgFailureErrors := orgFailure.GetErrors()
			if len(orgFailureErrors) > 0 && orgFailureErrors[0].Detail != nil {
				errorMessage = fmt.Sprintf("%s: %s", errorMessage, *orgFailureErrors[0].Detail)
			}
		}
	}

	if kafkaRestGenericOpenAPIError, ok := err.(kafkarestv3.GenericOpenAPIError); ok {
		if kafkaRestError, ok := kafkaRestGenericOpenAPIError.Model().(kafkarestv3.Error); ok {
			if kafkaRestError.Message != nil {
				errorMessage = fmt.Sprintf("%s: %s", errorMessage, *kafkaRestError.Message)
			}
		}
	}

	return diag.Diagnostics{
		diag.Diagnostic{
			Severity: diag.Error,
			Summary:  errorMessage,
		},
	}
}

// Reports whether the response has http.StatusForbidden status due to an invalid Cloud API Key vs other reasons
// which is useful to distinguish from scenarios where http.StatusForbidden represents http.StatusNotFound for
// security purposes.
func HasStatusForbiddenDueToInvalidAPIKey(response *http.Response) bool {
	if HasStatusForbidden(response) {
		bodyBytes, err := io.ReadAll(response.Body)
		if err != nil {
			return false
		}
		bodyString := string(bodyBytes)
		// Search for a specific error message that indicates the invalid Cloud API Key has been used
		return strings.Contains(bodyString, "invalid API key")
	}
	return false
}

// Reports whether the response has http.StatusForbidden status
func HasStatusForbidden(response *http.Response) bool {
	return response != nil && response.StatusCode == http.StatusForbidden
}

// Reports whether the response has http.StatusNotFound status
func HasStatusNotFound(response *http.Response) bool {
	return response != nil && response.StatusCode == http.StatusNotFound
}

// APIF-2043: TEMPORARY METHOD
// Converts principal with a resourceID (User:sa-01234) to principal with an integer ID (User:6789)
func principalWithResourceIdToPrincipalWithIntegerId(c *Client, principalWithResourceId string) (string, error) {
	// There's input validation that principal attribute must start with "User:sa-"
	// User:sa-abc123 -> sa-abc123
	resourceId := principalWithResourceId[5:]
	integerId, err := saResourceIdToSaIntegerId(c, resourceId)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%d", principalPrefix, integerId), nil
}

// APIF-2043: TEMPORARY METHOD
// Converts service account's resourceID (sa-abc123) to its integer ID (67890)
func saResourceIdToSaIntegerId(c *Client, saResourceId string) (int, error) {
	list, _, err := c.iamV1Client.ServiceAccountsV1Api.ListV1ServiceAccounts(c.iamV1ApiContext(context.Background())).Execute()
	if err != nil {
		return 0, err
	}
	for _, sa := range list.GetUsers() {
		if sa.GetResourceId() == saResourceId {
			if sa.HasId() {
				return int(sa.GetId()), nil
			} else {
				return 0, fmt.Errorf("the matching integer ID for a service account with resource ID=%s is nil", saResourceId)
			}
		}
	}
	return 0, fmt.Errorf("the service account with resource ID=%s was not found", saResourceId)
}

func clusterCrnToRbacClusterCrn(clusterCrn string) (string, error) {
	// Converts
	// crn://confluent.cloud/organization=./environment=./cloud-cluster=lkc-198rjz/kafka=lkc-198rjz
	// to
	// crn://confluent.cloud/organization=./environment=./cloud-cluster=lkc-198rjz
	lastIndex := strings.LastIndex(clusterCrn, crnKafkaSuffix)
	if lastIndex == -1 {
		return "", fmt.Errorf("could not find %s in %s", crnKafkaSuffix, clusterCrn)
	}
	return clusterCrn[:lastIndex], nil
}

func stringInSlice(target string, slice []string) bool {
	for _, value := range slice {
		if value == target {
			return true
		}
	}
	return false
}

func convertToStringStringMap(data map[string]interface{}) map[string]string {
	stringMap := make(map[string]string)

	for key, value := range data {
		stringMap[key] = value.(string)
	}

	return stringMap
}

func ptr(s string) *string {
	return &s
}
