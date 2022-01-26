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

func environmentDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: environmentDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type: schema.TypeString,
				// The id is required because an Environment ID must be set so the data source knows which Environment to retrieve.
				Required: true,
			},
			paramDisplayName: {
				Type:        schema.TypeString,
				Description: "A human-readable name for the Environment.",
				Computed:    true,
			},
		},
	}
}

func environmentDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	environmentId := d.Get(paramId).(string)
	log.Printf("[INFO] Environment read for %s", environmentId)
	c := meta.(*Client)
	environment, resp, err := executeEnvironmentRead(c.orgApiContext(ctx), c, environmentId)
	if err != nil {
		log.Printf("[ERROR] Environment get failed for id %s, %v, %s", environmentId, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDisplayName, environment.GetDisplayName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(environment.GetId())
	return nil
}
