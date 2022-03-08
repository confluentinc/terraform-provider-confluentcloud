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
	v2 "github.com/confluentinc/ccloud-sdk-go-v2/org/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
)

func environmentDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: environmentDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type:        schema.TypeString,
				Description: "The ID of the Environment (e.g., `env-abc123`).",
				Computed:    true,
				Optional:    true,
				// A user should provide a value for either "id" or "display_name" attribute
				ExactlyOneOf: []string{paramId, paramDisplayName},
			},
			paramDisplayName: {
				Type:         schema.TypeString,
				Description:  "A human-readable name for the Environment.",
				Computed:     true,
				Optional:     true,
				ExactlyOneOf: []string{paramId, paramDisplayName},
			},
		},
	}
}

func environmentDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// ExactlyOneOf specified in the schema ensures one of paramId or paramDisplayName is specified.
	// The next step is to figure out which one exactly is set.
	environmentId := d.Get(paramId).(string)
	displayName := d.Get(paramDisplayName).(string)

	if environmentId != "" {
		return environmentDataSourceReadUsingId(ctx, d, meta, environmentId)
	} else if displayName != "" {
		return environmentDataSourceReadUsingDisplayName(ctx, d, meta, displayName)
	} else {
		return diag.Errorf("error creating confluentcloud_environment data source: one of \"%s\" or \"%s\" must be specified but they're both empty", paramId, paramDisplayName)
	}
}

func environmentDataSourceReadUsingDisplayName(ctx context.Context, d *schema.ResourceData, meta interface{}, displayName string) diag.Diagnostics {
	log.Printf("[INFO] Environment read using \"%s\"=%s", paramDisplayName, displayName)

	c := meta.(*Client)
	environmentList, resp, err := c.orgClient.EnvironmentsOrgV2Api.ListOrgV2Environments(c.orgApiContext(ctx)).Execute()
	if err != nil {
		log.Printf("[ERROR] Environments get failed %v, %s", resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if orgHasMultipleEnvsWithTargetDisplayName(environmentList, displayName) {
		return diag.Errorf("There are multiple environments with display_name=%s", displayName)
	}

	for _, environment := range environmentList.GetData() {
		if environment.GetDisplayName() == displayName {
			return setEnvironmentDataSourceAttributes(d, environment)
		}
	}

	return diag.Errorf("The environment with display_name=%s was not found", displayName)
}

func environmentDataSourceReadUsingId(ctx context.Context, d *schema.ResourceData, meta interface{}, environmentId string) diag.Diagnostics {
	log.Printf("[INFO] Environment read using \"%s\"=%s", paramId, environmentId)

	c := meta.(*Client)
	environment, resp, err := executeEnvironmentRead(c.orgApiContext(ctx), c, environmentId)
	if err != nil {
		log.Printf("[ERROR] Environment get failed for id %s, %v, %s", environmentId, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	return setEnvironmentDataSourceAttributes(d, environment)
}

func setEnvironmentDataSourceAttributes(d *schema.ResourceData, environment v2.OrgV2Environment) diag.Diagnostics {
	if err := d.Set(paramDisplayName, environment.GetDisplayName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(environment.GetId())
	return nil
}

func orgHasMultipleEnvsWithTargetDisplayName(environmentList v2.OrgV2EnvironmentList, displayName string) bool {
	var numberOfEnvironmentsWithTargetDisplayName = 0
	for _, environment := range environmentList.GetData() {
		if environment.GetDisplayName() == displayName {
			numberOfEnvironmentsWithTargetDisplayName += 1
		}
	}
	return numberOfEnvironmentsWithTargetDisplayName > 1
}
