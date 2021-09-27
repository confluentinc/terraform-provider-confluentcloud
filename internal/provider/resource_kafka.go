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
	paramAvailability       = "availability"
	paramClusterType        = "cluster_type"
	paramBootStrapEndpoint  = "bootstrap_endpoint"
	paramHttpEndpoint       = "http_endpoint"
	kindBasicInLowercase    = "basic"
	kindStandardInLowercase = "standard"
	kindBasicCapitalized    = "Basic"
	kindStandardCapitalized = "Standard"

	stateInProgress = "in-progress"
	stateDone       = "done"

	waitUntilProvisioned        = "PROVISIONED"
	waitUntilBootstrapAvailable = "BOOTSTRAP_AVAILABLE"
	waitUntilNone               = "NONE"
)

var acceptedAvailabilityZones = []string{"single_zone", "multi_zone"}
var acceptedCloudProviders = []string{"aws", "azure", "gcp"}
var acceptedClusterTypes = []string{kindBasicInLowercase, kindStandardInLowercase}

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
			paramClusterType: {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The configuration of the Kafka cluster.",
				ValidateFunc: validation.StringInSlice(acceptedClusterTypes, false),
			},
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
			diag.FromErr(err)
		}
	}

	if d.HasChange(paramClusterType) {
		if clusterType == kindBasicInLowercase {
			return diag.FromErr(fmt.Errorf("clusters cannot be downgraded from 'standard' to 'basic'"))
		}
		updateReq := cmk.NewCmkV2ClusterUpdate()
		updateSpec := cmk.NewCmkV2ClusterSpecUpdate()
		updateSpec.SetConfig(cmk.CmkV2StandardAsCmkV2ClusterSpecUpdateConfigOneOf(cmk.NewCmkV2Standard(kindStandardInLowercase)))
		updateSpec.SetEnvironment(cmk.ObjectReference{Id: environmentId})
		updateReq.SetSpec(*updateSpec)
		req := c.cmkClient.ClustersCmkV2Api.UpdateCmkV2Cluster(c.cmkApiContext(ctx), d.Id()).CmkV2ClusterUpdate(*updateReq)

		cluster, _, err := req.Execute()

		if err == nil {
			if cluster.Spec.Config.CmkV2Basic != nil {
				err = d.Set(paramClusterType, kindBasicInLowercase)
			} else if cluster.Spec.Config.CmkV2Standard != nil {
				err = d.Set(paramClusterType, kindStandardInLowercase)
			}
		}

		if err != nil {
			diag.FromErr(err)
		}
	}

	return nil
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
	if clusterType == kindBasicCapitalized {
		spec.SetConfig(cmk.CmkV2BasicAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Basic(kindBasicCapitalized)))
	} else if clusterType == kindStandardCapitalized {
		spec.SetConfig(cmk.CmkV2StandardAsCmkV2ClusterSpecConfigOneOf(cmk.NewCmkV2Standard(kindStandardCapitalized)))
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
		Timeout:      120 * time.Minute,
		Delay:        3 * time.Second,
		PollInterval: 1 * time.Minute,
		MinTimeout:   120 * time.Second,
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
			return nil, "UNKNOWN", err
		}

		jsonCluster, _ := cluster.MarshalJSON()
		log.Printf("[DEBUG] Kafka cluster %s", jsonCluster)

		if strings.ToUpper(c.waitUntil) == waitUntilProvisioned {
			log.Printf("[DEBUG] Waiting for Kafka cluster to be PROVISIONED: current status %s", cluster.Status.GetPhase())
			if cluster.Status.GetPhase() == "PROVISIONING" {
				return cluster, stateInProgress, nil
			}
			return cluster, stateDone, nil
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

func extractAvailability(d *schema.ResourceData) string {
	availability := strings.ToUpper(d.Get(paramAvailability).(string))
	return availability
}

func extractRegion(d *schema.ResourceData) string {
	region := d.Get(paramRegion).(string)
	return region
}

func extractCloud(d *schema.ResourceData) string {
	cloud := strings.ToUpper(d.Get(paramCloud).(string))
	return cloud
}

func extractClusterType(d *schema.ResourceData) string {
	clusterType := strings.Title(strings.ToLower(d.Get(paramClusterType).(string)))
	return clusterType
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
		err = d.Set(paramAvailability, strings.ToLower(cluster.Spec.GetAvailability()))
	}
	if err == nil {
		err = d.Set(paramCloud, strings.ToLower(cluster.Spec.GetCloud()))
	}
	if err == nil {
		err = d.Set(paramRegion, cluster.Spec.GetRegion())
	}

	if err == nil {
		if cluster.Spec.Config.CmkV2Basic != nil {
			err = d.Set(paramClusterType, kindBasicInLowercase)
		} else if cluster.Spec.Config.CmkV2Standard != nil {
			err = d.Set(paramClusterType, kindStandardInLowercase)
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
	}

	err = d.Set(paramDisplayName, cluster.Spec.GetDisplayName())
	if err == nil {
		err = d.Set(paramAvailability, strings.ToLower(cluster.Spec.GetAvailability()))
	}
	if err == nil {
		err = d.Set(paramCloud, strings.ToLower(cluster.Spec.GetCloud()))
	}
	if err == nil {
		err = d.Set(paramRegion, cluster.Spec.GetRegion())
	}

	if err == nil {
		if cluster.Spec.Config.CmkV2Basic != nil {
			err = d.Set(paramClusterType, kindBasicInLowercase)
		} else if cluster.Spec.Config.CmkV2Standard != nil {
			err = d.Set(paramClusterType, kindStandardInLowercase)
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
