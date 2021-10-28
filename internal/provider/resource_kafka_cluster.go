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
			paramEnvironment: environmentSchema(),
		},
	}
}

func kafkaUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := extractDisplayName(d)
	environmentId, err := validEnvironmentId(c, d)
	if err != nil {
		return diag.FromErr(err)
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

		cluster, _, err := req.Execute()

		if err == nil {
			err = d.Set(paramDisplayName, cluster.Spec.DisplayName)
		}

		if err != nil {
			return diag.FromErr(err)
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

		cluster, _, err := req.Execute()

		if err == nil && cluster.Spec.Config.CmkV2Standard != nil {
			err = d.Set(paramStandardCluster, []interface{}{make(map[string]string)})
		}

		if err != nil {
			return diag.FromErr(err)
		}
	} else if isForbiddenStandardBasicDowngrade || isForbiddenDedicatedUpdate {
		// Revert the cluster type in TF state
		log.Printf("[WARN] Reverting Kafka cluster (%s) type", d.Id())
		oldBasicClusterConfig, _ := d.GetChange(paramBasicCluster)
		oldStandardClusterConfig, _ := d.GetChange(paramStandardCluster)
		oldDedicatedClusterConfig, _ := d.GetChange(paramDedicatedCluster)
		_ = d.Set(paramBasicCluster, oldBasicClusterConfig)
		_ = d.Set(paramStandardCluster, oldStandardClusterConfig)
		_ = d.Set(paramDedicatedCluster, oldDedicatedClusterConfig)
		return diag.FromErr(fmt.Errorf("clusters can only be upgraded from 'Basic' to 'Standard'"))
	}

	isCkuUpdate := d.HasChange(paramDedicatedCluster) && clusterType == kafkaClusterTypeDedicated && d.HasChange(paramDedicatedCku)
	if isCkuUpdate {
		oldCku, newCku := d.GetChange(paramDedicatedCku)
		if newCku.(int) < oldCku.(int) {
			// decreasing the number of CKUs aka Kafka Shrink operation
			if newCku.(int)+1 != oldCku.(int) {
				return diag.FromErr(fmt.Errorf("decreasing the number of CKUs by more than 1 is currently not supported"))
			}
		}
		availability := extractAvailability(d)
		err = ckuCheck(cku, availability)
		if err != nil {
			return diag.FromErr(err)
		}

		updateReq := cmk.NewCmkV2ClusterUpdate()
		updateSpec := cmk.NewCmkV2ClusterSpecUpdate()
		updateSpec.SetConfig(cmk.CmkV2DedicatedAsCmkV2ClusterSpecUpdateConfigOneOf(cmk.NewCmkV2Dedicated(kafkaClusterTypeDedicated, cku)))
		updateSpec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
		updateReq.SetSpec(*updateSpec)
		req := c.cmkClient.ClustersCmkV2Api.UpdateCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).CmkV2ClusterUpdate(*updateReq)

		_, _, err := req.Execute()
		if err != nil {
			return diag.FromErr(err)
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
		output, err := stateConf.WaitForStateContext(c.cmkApiContext(ctx))
		if err != nil {
			return diag.FromErr(fmt.Errorf("error waiting for CKU update of Kafka cluster (%s): %s", d.Id(), err))
		}
		err = d.Set(paramDedicatedCluster, []interface{}{map[string]interface{}{
			paramCku: output.(cmk.CmkV2Cluster).Status.Cku,
		}})
		if err != nil {
			return diag.FromErr(err)
		}
	}

	return diag.FromErr(err)
}

func executeKafkaCreate(ctx context.Context, c *Client, cluster *cmk.CmkV2Cluster) (cmk.CmkV2Cluster, *http.Response, error) {
	req := c.cmkClient.ClustersCmkV2Api.CreateCmkV2Cluster(c.cmkApiContext(ctx)).CmkV2Cluster(*cluster)

	return req.Execute()
}

func kafkaCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := extractDisplayName(d)
	availability := extractAvailability(d)
	cloud := extractCloud(d)
	region := extractRegion(d)
	clusterType := extractClusterType(d)
	environmentId, err := validEnvironmentId(c, d)
	if err != nil {
		return diag.FromErr(err)
	}
	err = setEnvironmentId(environmentId, d)
	if err != nil {
		return diag.FromErr(err)
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
			return diag.FromErr(err)
		}
		spec.SetConfig(cmk.CmkV2DedicatedAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Dedicated(kafkaClusterTypeDedicated, cku)))
	} else {
		log.Printf("[ERROR] Creating Kafka cluster create failed: unknown Kafka cluster type was provided: %s", clusterType)
		return diag.FromErr(fmt.Errorf("kafka cluster create failed: unknown Kafka cluster type was provided: %s", clusterType))
	}
	spec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
	cluster := cmk.CmkV2Cluster{Spec: spec}

	specBytes, err := json.Marshal(spec)
	if err != nil {
		log.Printf("[ERROR] JSON marshaling failed on spec: %s", err)
		return diag.FromErr(err)
	}
	log.Printf("[DEBUG] Creating Kafka cluster with spec %s", specBytes)

	kafka, resp, err := executeKafkaCreate(c.cmkApiContext(ctx), c, &cluster)
	if err != nil {
		log.Printf("[ERROR] Kafka cluster create failed %v, %v, %s", cluster, resp, err)
		return diag.FromErr(err)
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
	output, err := stateConf.WaitForStateContext(c.cmkApiContext(ctx))
	if err != nil {
		return diag.FromErr(fmt.Errorf("error waiting for Kafka cluster (%s) to be %s: %s", d.Id(), err, stateDone))
	}

	if err == nil {
		err = d.Set(paramBootStrapEndpoint, output.(cmk.CmkV2Cluster).Spec.GetKafkaBootstrapEndpoint())
	}
	if err == nil {
		err = d.Set(paramHttpEndpoint, output.(cmk.CmkV2Cluster).Spec.GetHttpEndpoint())
	}
	return diag.FromErr(err)
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
		// https://confluentinc.atlassian.net/browse/CAPAC-293
		if cluster.Status.GetCku() == cluster.Spec.Config.CmkV2Dedicated.Cku && cluster.Status.GetCku() == desiredCku {
			return cluster, stateDone, nil
		}
		return cluster, stateInProgress, nil
	}
}

func extractAvailability(d *schema.ResourceData) string {
	availability := d.Get(paramAvailability).(string)
	return availability
}

func extractRegion(d *schema.ResourceData) string {
	region := d.Get(paramRegion).(string)
	return region
}

func extractCloud(d *schema.ResourceData) string {
	cloud := d.Get(paramCloud).(string)
	return cloud
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

	environmentId, err := validEnvironmentId(c, d)
	if err != nil {
		return diag.FromErr(err)
	}

	req := c.cmkClient.ClustersCmkV2Api.DeleteCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).Environment(environmentId)
	_, err = req.Execute()

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting Kafka cluster (%s), err: %s", d.Id(), err))
	}

	return nil
}

func kafkaImport(ctx context.Context, d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	envIDAndClusterID := d.Id()
	parts := strings.Split(envIDAndClusterID, "/")

	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid format for kafka import: expected '<env ID>/<lkc ID>'")
	}

	c := meta.(*Client)
	cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, parts[0], parts[1])
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	d.SetId(parts[1])
	err = setEnvironmentId(parts[0], d)
	if err != nil {
		return nil, err
	}

	err = d.Set(paramDisplayName, cluster.Spec.GetDisplayName())
	if err == nil {
		err = d.Set(paramAvailability, cluster.Spec.GetAvailability())
	}
	if err == nil {
		err = d.Set(paramCloud, cluster.Spec.GetCloud())
	}
	if err == nil {
		err = d.Set(paramRegion, cluster.Spec.GetRegion())
	}

	if err == nil {
		if cluster.Spec.Config.CmkV2Basic != nil {
			err = d.Set(paramBasicCluster, []interface{}{make(map[string]string)})
		} else if cluster.Spec.Config.CmkV2Standard != nil {
			err = d.Set(paramStandardCluster, []interface{}{make(map[string]string)})
		} else if cluster.Spec.Config.CmkV2Dedicated != nil {
			err = d.Set(paramDedicatedCluster, []interface{}{map[string]interface{}{
				paramCku: cluster.Status.Cku,
			}})
		}
	}

	if err == nil {
		err = d.Set(paramBootStrapEndpoint, cluster.Spec.GetKafkaBootstrapEndpoint())
	}
	if err == nil {
		err = d.Set(paramHttpEndpoint, cluster.Spec.GetHttpEndpoint())
	}
	if err == nil {
		err = setEnvironmentId(parts[0], d)
	}

	return []*schema.ResourceData{d}, err
}

func executeKafkaRead(ctx context.Context, c *Client, environmentId string, clusterId string) (cmk.CmkV2Cluster, *http.Response, error) {
	req := c.cmkClient.ClustersCmkV2Api.GetCmkV2Cluster(c.cmkApiContext(ctx), clusterId).Environment(environmentId)
	return req.Execute()
}

func kafkaRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Kafka read for %s", d.Id())
	c := meta.(*Client)
	environmentId, err := validEnvironmentId(c, d)
	if err != nil {
		log.Printf("[ERROR] %s", err)
		return diag.FromErr(err)
	}
	cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, environmentId, d.Id())
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if err != nil {
		log.Printf("[ERROR] Kafka cluster get failed for id %s, %v, %s", d.Id(), resp, err)
		return diag.FromErr(err)
	}

	err = d.Set(paramDisplayName, cluster.Spec.GetDisplayName())
	if err == nil {
		err = d.Set(paramAvailability, cluster.Spec.GetAvailability())
	}
	if err == nil {
		err = d.Set(paramCloud, cluster.Spec.GetCloud())
	}
	if err == nil {
		err = d.Set(paramRegion, cluster.Spec.GetRegion())
	}

	if err == nil {
		if cluster.Spec.Config.CmkV2Basic != nil {
			err = d.Set(paramBasicCluster, []interface{}{make(map[string]string)})
		} else if cluster.Spec.Config.CmkV2Standard != nil {
			err = d.Set(paramStandardCluster, []interface{}{make(map[string]string)})
		} else if cluster.Spec.Config.CmkV2Dedicated != nil {
			err = d.Set(paramDedicatedCluster, []interface{}{map[string]interface{}{
				paramCku: cluster.Status.Cku,
			}})
		}
	}

	if err == nil {
		err = d.Set(paramBootStrapEndpoint, cluster.Spec.GetKafkaBootstrapEndpoint())
	}
	if err == nil {
		err = d.Set(paramHttpEndpoint, cluster.Spec.GetHttpEndpoint())
	}
	if err == nil {
		err = setEnvironmentId(environmentId, d)
	}
	return diag.FromErr(err)
}

func basicClusterSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MinItems: 0,
		MaxItems: 1,
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
		MinItems: 0,
		MaxItems: 1,
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
		MinItems: 0,
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
