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
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"

	cmk "github.com/confluentinc/ccloud-sdk-go-v2/cmk/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

const (
	kafkaClusterTypeBasic     = "Basic"
	kafkaClusterTypeStandard  = "Standard"
	kafkaClusterTypeDedicated = "Dedicated"
	paramBasicCluster         = "basic"
	paramStandardCluster      = "standard"
	paramDedicatedCluster     = "dedicated"
	paramAvailability         = "availability"
	paramBootStrapEndpoint    = "bootstrap_endpoint"
	paramHttpEndpoint         = "http_endpoint"
	paramCku                  = "cku"
	paramRbacCrn              = "rbac_crn"

	stateInProgress = "in-progress"
	stateDone       = "done"
	stateFailed     = "FAILED"
	stateUnknown    = "UNKNOWN"

	waitUntilProvisioned        = "PROVISIONED"
	waitUntilBootstrapAvailable = "BOOTSTRAP_AVAILABLE"
	waitUntilNone               = "NONE"

	singleZone = "SINGLE_ZONE"
	multiZone  = "MULTI_ZONE"
)

var acceptedAvailabilityZones = []string{singleZone, multiZone}
var acceptedCloudProviders = []string{"AWS", "AZURE", "GCP"}
var acceptedClusterTypes = []string{paramBasicCluster, paramStandardCluster, paramDedicatedCluster}
var paramDedicatedCku = fmt.Sprintf("%s.0.%s", paramDedicatedCluster, paramCku)

func kafkaResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: kafkaCreate,
		ReadContext:   kafkaRead,
		UpdateContext: kafkaUpdate,
		DeleteContext: kafkaDelete,
		Importer: &schema.ResourceImporter{
			StateContext: kafkaImport,
		},
		Schema: map[string]*schema.Schema{
			paramDisplayName: {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The name of the Kafka cluster.",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			paramApiVersion: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "API Version defines the schema version of this representation of a Kafka cluster.",
			},
			paramKind: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Kind defines the object Kafka cluster represents.",
			},
			paramAvailability: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The availability zone configuration of the Kafka cluster.",
				ValidateFunc: validation.StringInSlice(acceptedAvailabilityZones, false),
			},
			paramCloud: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The cloud service provider that runs the Kafka cluster.",
				ValidateFunc: validation.StringInSlice(acceptedCloudProviders, false),
			},
			paramRegion: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The cloud service provider region where the Kafka cluster is running.",
			},
			paramBasicCluster:     basicClusterSchema(),
			paramStandardCluster:  standardClusterSchema(),
			paramDedicatedCluster: dedicatedClusterSchema(),
			paramBootStrapEndpoint: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The bootstrap endpoint used by Kafka clients to connect to the Kafka cluster.",
			},
			paramHttpEndpoint: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The REST endpoint of the Kafka cluster.",
			},
			paramRbacCrn: {
				Type:     schema.TypeString,
				Computed: true,
				Description: "The Confluent Resource Name of the Kafka cluster suitable for " +
					"confluentcloud_role_binding's crn_pattern.",
			},
			paramEnvironment: environmentSchema(),
		},
	}
}

func kafkaUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := d.Get(paramDisplayName).(string)
	environmentId, err := validEnvironmentId(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	clusterType := extractClusterType(d)
	// Non-zero value means CKU has been set
	cku := extractCku(d)
	if d.HasChange(paramDisplayName) {
		updateReq := cmk.NewCmkV2ClusterUpdate()
		updateSpec := cmk.NewCmkV2ClusterSpecUpdate()
		updateSpec.SetDisplayName(displayName)
		updateSpec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
		updateReq.SetSpec(*updateSpec)
		req := c.cmkClient.ClustersCmkV2Api.UpdateCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).CmkV2ClusterUpdate(*updateReq)

		_, _, err := req.Execute()

		if err != nil {
			return createDiagnosticsWithDetails(err)
		}
	}

	// Allow only Basic -> Standard upgrade
	isBasicStandardUpdate := d.HasChange(paramBasicCluster) && d.HasChange(paramStandardCluster) && !d.HasChange(paramDedicatedCluster) && clusterType == kafkaClusterTypeStandard
	// Watch out for forbidden updates / downgrades: e.g., Standard -> Basic, Basic -> Dedicated etc.
	isForbiddenStandardBasicDowngrade := d.HasChange(paramBasicCluster) && d.HasChange(paramStandardCluster) && !d.HasChange(paramDedicatedCluster) && clusterType == kafkaClusterTypeBasic
	isForbiddenDedicatedUpdate := d.HasChange(paramDedicatedCluster) && (d.HasChange(paramBasicCluster) || d.HasChange(paramStandardCluster))

	if isBasicStandardUpdate {
		updateReq := cmk.NewCmkV2ClusterUpdate()
		updateSpec := cmk.NewCmkV2ClusterSpecUpdate()
		updateSpec.SetConfig(cmk.CmkV2StandardAsCmkV2ClusterSpecUpdateConfigOneOf(cmk.NewCmkV2Standard(kafkaClusterTypeStandard)))
		updateSpec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
		updateReq.SetSpec(*updateSpec)
		req := c.cmkClient.ClustersCmkV2Api.UpdateCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).CmkV2ClusterUpdate(*updateReq)

		_, _, err := req.Execute()

		if err != nil {
			return createDiagnosticsWithDetails(err)
		}
	} else if isForbiddenStandardBasicDowngrade || isForbiddenDedicatedUpdate {
		return diag.Errorf("clusters can only be upgraded from 'Basic' to 'Standard'")
	}

	isCkuUpdate := d.HasChange(paramDedicatedCluster) && clusterType == kafkaClusterTypeDedicated && d.HasChange(paramDedicatedCku)
	if isCkuUpdate {
		oldCku, newCku := d.GetChange(paramDedicatedCku)
		if newCku.(int) < oldCku.(int) {
			// decreasing the number of CKUs aka Kafka Shrink operation
			if newCku.(int)+1 != oldCku.(int) {
				return diag.Errorf("decreasing the number of CKUs by more than 1 is currently not supported")
			}
		}
		availability := d.Get(paramAvailability).(string)
		err = ckuCheck(cku, availability)
		if err != nil {
			return createDiagnosticsWithDetails(err)
		}

		updateReq := cmk.NewCmkV2ClusterUpdate()
		updateSpec := cmk.NewCmkV2ClusterSpecUpdate()
		updateSpec.SetConfig(cmk.CmkV2DedicatedAsCmkV2ClusterSpecUpdateConfigOneOf(cmk.NewCmkV2Dedicated(kafkaClusterTypeDedicated, cku)))
		updateSpec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
		updateReq.SetSpec(*updateSpec)
		req := c.cmkClient.ClustersCmkV2Api.UpdateCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).CmkV2ClusterUpdate(*updateReq)

		_, _, err := req.Execute()
		if err != nil {
			return createDiagnosticsWithDetails(err)
		}

		stateConf := &resource.StateChangeConf{
			Pending:      []string{stateInProgress},
			Target:       []string{stateDone},
			Refresh:      kafkaCkuUpdated(c.cmkApiContext(ctx), c, environmentId, d.Id(), cku),
			Timeout:      24 * time.Hour,
			Delay:        5 * time.Second,
			PollInterval: 1 * time.Minute,
		}

		log.Printf("[DEBUG] Waiting for Kafka cluster CKU update to complete")
		_, err = stateConf.WaitForStateContext(c.cmkApiContext(ctx))
		if err != nil {
			return diag.Errorf("error waiting for CKU update of Kafka cluster (%s): %s", d.Id(), err)
		}
	}

	return kafkaRead(ctx, d, meta)
}

func executeKafkaCreate(ctx context.Context, c *Client, cluster *cmk.CmkV2Cluster) (cmk.CmkV2Cluster, *http.Response, error) {
	req := c.cmkClient.ClustersCmkV2Api.CreateCmkV2Cluster(c.cmkApiContext(ctx)).CmkV2Cluster(*cluster)

	return req.Execute()
}

func kafkaCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := d.Get(paramDisplayName).(string)
	availability := d.Get(paramAvailability).(string)
	cloud := d.Get(paramCloud).(string)
	region := d.Get(paramRegion).(string)
	clusterType := extractClusterType(d)
	environmentId, err := validEnvironmentId(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}
	err = setEnvironmentId(environmentId, d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}

	spec := cmk.NewCmkV2ClusterSpec()
	spec.SetDisplayName(displayName)
	spec.SetAvailability(availability)
	spec.SetCloud(cloud)
	spec.SetRegion(region)
	if clusterType == kafkaClusterTypeBasic {
		spec.SetConfig(cmk.CmkV2BasicAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Basic(kafkaClusterTypeBasic)))
	} else if clusterType == kafkaClusterTypeStandard {
		spec.SetConfig(cmk.CmkV2StandardAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Standard(kafkaClusterTypeStandard)))
	} else if clusterType == kafkaClusterTypeDedicated {
		cku := extractCku(d)
		err = ckuCheck(cku, availability)
		if err != nil {
			return createDiagnosticsWithDetails(err)
		}
		spec.SetConfig(cmk.CmkV2DedicatedAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Dedicated(kafkaClusterTypeDedicated, cku)))
	} else {
		log.Printf("[ERROR] Creating Kafka cluster create failed: unknown Kafka cluster type was provided: %s", clusterType)
		return diag.Errorf("kafka cluster create failed: unknown Kafka cluster type was provided: %s", clusterType)
	}
	spec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
	cluster := cmk.CmkV2Cluster{Spec: spec}

	specBytes, err := json.Marshal(spec)
	if err != nil {
		log.Printf("[ERROR] JSON marshaling failed on spec: %s", err)
		return createDiagnosticsWithDetails(err)
	}
	log.Printf("[DEBUG] Creating Kafka cluster with spec %s", specBytes)

	kafka, resp, err := executeKafkaCreate(c.cmkApiContext(ctx), c, &cluster)
	if err != nil {
		log.Printf("[ERROR] Kafka cluster create failed %v, %v, %s", cluster, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(kafka.GetId())
	log.Printf("[DEBUG] Created cluster %s", kafka.GetId())

	stateConf := &resource.StateChangeConf{
		Pending:      []string{stateInProgress},
		Target:       []string{stateDone},
		Refresh:      kafkaProvisioned(c.cmkApiContext(ctx), c, environmentId, d.Id()),
		Timeout:      getTimeoutFor(clusterType),
		Delay:        5 * time.Second,
		PollInterval: 1 * time.Minute,
	}

	log.Printf("[DEBUG] Waiting for Kafka cluster provisioning to become %s", stateDone)
	_, err = stateConf.WaitForStateContext(c.cmkApiContext(ctx))
	if err != nil {
		return diag.Errorf("error waiting for Kafka cluster (%s) to be %s: %s", d.Id(), err, stateDone)
	}

	return kafkaRead(ctx, d, meta)
}

func kafkaProvisioned(ctx context.Context, c *Client, environmentId string, clusterId string) resource.StateRefreshFunc {
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

func kafkaCkuUpdated(ctx context.Context, c *Client, environmentId string, clusterId string, desiredCku int32) resource.StateRefreshFunc {
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

func extractClusterType(d *schema.ResourceData) string {
	basicConfigBlock := d.Get(paramBasicCluster).([]interface{})
	standardConfigBlock := d.Get(paramStandardCluster).([]interface{})
	dedicatedConfigBlock := d.Get(paramDedicatedCluster).([]interface{})

	if len(basicConfigBlock) == 1 {
		return kafkaClusterTypeBasic
	} else if len(standardConfigBlock) == 1 {
		return kafkaClusterTypeStandard
	} else if len(dedicatedConfigBlock) == 1 {
		return kafkaClusterTypeDedicated
	}
	return ""
}

func extractCku(d *schema.ResourceData) int32 {
	// CKUs are only defined for dedicated clusters
	if kafkaClusterTypeDedicated != extractClusterType(d) {
		return 0
	}

	// d.Get() will return 0 if the key is not present
	return int32(d.Get(paramDedicatedCku).(int))
}

func kafkaDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	environmentId, err := validEnvironmentId(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}

	req := c.cmkClient.ClustersCmkV2Api.DeleteCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).Environment(environmentId)
	_, err = req.Execute()

	if err != nil {
		return diag.Errorf("error deleting Kafka cluster (%s), err: %s", d.Id(), err)
	}

	return nil
}

func kafkaImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	envIDAndClusterID := d.Id()
	parts := strings.Split(envIDAndClusterID, "/")

	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for kafka import: expected '<env ID>/<lkc ID>'")
	}

	environmentId := parts[0]
	clusterId := parts[1]
	d.SetId(clusterId)
	log.Printf("[INFO] Kafka import for %s", clusterId)

	return readAndSetResourceConfigurationArguments(ctx, d, meta, environmentId, clusterId)
}

func executeKafkaRead(ctx context.Context, c *Client, environmentId string, clusterId string) (cmk.CmkV2Cluster, *http.Response, error) {
	req := c.cmkClient.ClustersCmkV2Api.GetCmkV2Cluster(c.cmkApiContext(ctx), clusterId).Environment(environmentId)
	return req.Execute()
}

func kafkaRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clusterId := d.Id()
	log.Printf("[INFO] Kafka read for %s", clusterId)
	environmentId, err := validEnvironmentId(d)
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return createDiagnosticsWithDetails(err)
	}

	_, err = readAndSetResourceConfigurationArguments(ctx, d, meta, environmentId, clusterId)

	return createDiagnosticsWithDetails(err)
}

func readAndSetResourceConfigurationArguments(ctx context.Context, d *schema.ResourceData, meta interface{}, environmentId, clusterId string) ([]*schema.ResourceData, error) {
	c := meta.(*Client)

	cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, environmentId, clusterId)
	if err != nil {
		log.Printf("[ERROR] Kafka cluster get failed for id %s, %v, %s", clusterId, resp, err)

		// https://learn.hashicorp.com/tutorials/terraform/provider-setup
		isResourceNotFound := HasStatusForbidden(resp) && !HasStatusForbiddenDueToInvalidAPIKey(resp)
		if isResourceNotFound {
			log.Printf("[WARN] Kafka cluster with id=%s is not found", d.Id())
			// If the resource isn't available, Terraform destroys the resource in state.
			d.SetId("")
			return nil, nil
		}

		return nil, err
	}

	if err := d.Set(paramApiVersion, cluster.GetApiVersion()); err != nil {
		return nil, err
	}
	if err := d.Set(paramKind, cluster.GetKind()); err != nil {
		return nil, err
	}
	if err := d.Set(paramDisplayName, cluster.Spec.GetDisplayName()); err != nil {
		return nil, err
	}
	if err := d.Set(paramAvailability, cluster.Spec.GetAvailability()); err != nil {
		return nil, err
	}
	if err := d.Set(paramCloud, cluster.Spec.GetCloud()); err != nil {
		return nil, err
	}
	if err := d.Set(paramRegion, cluster.Spec.GetRegion()); err != nil {
		return nil, err
	}

	// Reset all 3 cluster types since only one of these 3 should be set
	if err := d.Set(paramBasicCluster, []interface{}{}); err != nil {
		return nil, err
	}
	if err := d.Set(paramStandardCluster, []interface{}{}); err != nil {
		return nil, err
	}
	if err := d.Set(paramDedicatedCluster, []interface{}{}); err != nil {
		return nil, err
	}

	// Set a specific cluster type
	if cluster.Spec.Config.CmkV2Basic != nil {
		if err := d.Set(paramBasicCluster, []interface{}{make(map[string]string)}); err != nil {
			return nil, err
		}
	} else if cluster.Spec.Config.CmkV2Standard != nil {
		if err := d.Set(paramStandardCluster, []interface{}{make(map[string]string)}); err != nil {
			return nil, err
		}
	} else if cluster.Spec.Config.CmkV2Dedicated != nil {
		if err := d.Set(paramDedicatedCluster, []interface{}{map[string]interface{}{
			paramCku: cluster.Status.Cku,
		}}); err != nil {
			return nil, err
		}
	}

	if err := d.Set(paramBootStrapEndpoint, cluster.Spec.GetKafkaBootstrapEndpoint()); err != nil {
		return nil, err
	}
	if err := d.Set(paramHttpEndpoint, cluster.Spec.GetHttpEndpoint()); err != nil {
		return nil, err
	}
	rbacCrn, err := clusterCrnToRbacClusterCrn(cluster.Metadata.GetResourceName())
	if err != nil {
		log.Printf("[ERROR] Could not construct %s for kafka cluster with id=%s", paramRbacCrn, d.Id())
		return nil, err
	}
	if err := d.Set(paramRbacCrn, rbacCrn); err != nil {
		return nil, err
	}
	if err := setEnvironmentId(environmentId, d); err != nil {
		return nil, err
	}

	return []*schema.ResourceData{d}, nil
}

func basicClusterSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 0,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
		ExactlyOneOf: acceptedClusterTypes,
	}
}

func standardClusterSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 0,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
		ExactlyOneOf: acceptedClusterTypes,
	}
}

func dedicatedClusterSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramCku: {
					Type:        schema.TypeInt,
					Required:    true,
					Description: "The number of Confluent Kafka Units (CKUs) for Dedicated cluster types. MULTI_ZONE dedicated clusters must have at least two CKUs.",
					// TODO: add validation for CKUs >= 2 of MULTI_ZONE dedicated clusters
					ValidateFunc: validation.IntAtLeast(1),
				},
			},
		},
		ExactlyOneOf: acceptedClusterTypes,
	}
}

func ckuCheck(cku int32, availability string) error {
	if cku < 1 && availability == singleZone {
		return fmt.Errorf("single-zone dedicated clusters must have at least 1 CKU")
	} else if cku < 2 && availability == multiZone {
		return fmt.Errorf("multi-zone dedicated clusters must have at least 2 CKUs")
	}
	return nil
}
