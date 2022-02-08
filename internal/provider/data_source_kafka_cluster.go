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

func kafkaDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: kafkaDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type: schema.TypeString,
				// The id is required because a Kafka cluster ID must be set
				// so the data source knows which Kafka cluster to retrieve.
				Required: true,
			},
			// Similarly, paramEnvironment is required as well
			paramEnvironment: environmentDataSourceSchema(),
			paramApiVersion: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramKind: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramDisplayName: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramAvailability: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramCloud: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramRegion: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramBasicCluster:     basicClusterDataSourceSchema(),
			paramStandardCluster:  standardClusterDataSourceSchema(),
			paramDedicatedCluster: dedicatedClusterDataSourceSchema(),
			paramBootStrapEndpoint: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramHttpEndpoint: {
				Type:     schema.TypeString,
				Computed: true,
			},
			paramRbacCrn: {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func kafkaDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clusterId := d.Get(paramId).(string)
	log.Printf("[INFO] Kafka cluster read for %s", clusterId)

	environmentId, err := validEnvironmentId(d)
	if err != nil {
		return createDiagnosticsWithDetails(err)
	}

	// Copied from readAndSetResourceConfigurationArguments excluding logic around 404
	// TODO: move shared piece of code into a separate method
	c := meta.(*Client)

	cluster, resp, err := executeKafkaRead(c.cmkApiContext(ctx), c, environmentId, clusterId)
	if err != nil {
		log.Printf("[ERROR] Kafka cluster get failed for id %s, %v, %s", clusterId, resp, err)
		return createDiagnosticsWithDetails(err)
	}

	if err := d.Set(paramApiVersion, cluster.GetApiVersion()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramKind, cluster.GetKind()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDisplayName, cluster.Spec.GetDisplayName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramAvailability, cluster.Spec.GetAvailability()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramCloud, cluster.Spec.GetCloud()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramRegion, cluster.Spec.GetRegion()); err != nil {
		return createDiagnosticsWithDetails(err)
	}

	// Reset all 3 cluster types since only one of these 3 should be set
	if err := d.Set(paramBasicCluster, []interface{}{}); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramStandardCluster, []interface{}{}); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDedicatedCluster, []interface{}{}); err != nil {
		return createDiagnosticsWithDetails(err)
	}

	// Set a specific cluster type
	if cluster.Spec.Config.CmkV2Basic != nil {
		if err := d.Set(paramBasicCluster, []interface{}{make(map[string]string)}); err != nil {
			return createDiagnosticsWithDetails(err)
		}
	} else if cluster.Spec.Config.CmkV2Standard != nil {
		if err := d.Set(paramStandardCluster, []interface{}{make(map[string]string)}); err != nil {
			return createDiagnosticsWithDetails(err)
		}
	} else if cluster.Spec.Config.CmkV2Dedicated != nil {
		if err := d.Set(paramDedicatedCluster, []interface{}{map[string]interface{}{
			paramCku: cluster.Status.Cku,
		}}); err != nil {
			return createDiagnosticsWithDetails(err)
		}
	}

	if err := d.Set(paramBootStrapEndpoint, cluster.Spec.GetKafkaBootstrapEndpoint()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramHttpEndpoint, cluster.Spec.GetHttpEndpoint()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	rbacCrn, err := clusterCrnToRbacClusterCrn(cluster.Metadata.GetResourceName())
	if err != nil {
		log.Printf("[ERROR] Could not construct %s for kafka cluster with id=%s", paramRbacCrn, clusterId)
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramRbacCrn, rbacCrn); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := setEnvironmentId(environmentId, d); err != nil {
		return createDiagnosticsWithDetails(err)
	}

	d.SetId(cluster.GetId())
	return nil
}

func basicClusterDataSourceSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 0,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	}
}

func standardClusterDataSourceSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 0,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{},
		},
	}
}

func dedicatedClusterDataSourceSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		Optional: true,
		MaxItems: 1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				paramCku: {
					Type:        schema.TypeInt,
					Computed:    true,
					Description: "The number of Confluent Kafka Units (CKUs) for Dedicated cluster types. MULTI_ZONE dedicated clusters must have at least two CKUs.",
				},
			},
		},
	}
}
