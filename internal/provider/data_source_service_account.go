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

func serviceAccountDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: serviceAccountDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type: schema.TypeString,
				// The id is required because an Service Account ID must be set
				// so the data source knows which Service Account to retrieve.
				Required: true,
			},
			paramDisplayName: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "A human-readable name for the Service Account.",
			},
			paramDescription: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "A free-form description of the Service Account.",
			},
		},
	}
}

func serviceAccountDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	serviceAccountId := d.Get(paramId).(string)
	log.Printf("[INFO] Service account read for %s", serviceAccountId)
	c := meta.(*Client)
	serviceAccount, resp, err := executeServiceAccountRead(c.iamApiContext(ctx), c, serviceAccountId)
	if err != nil {
		log.Printf("[ERROR] Service account get failed for id %s, %v, %s", serviceAccountId, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDisplayName, serviceAccount.GetDisplayName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDescription, serviceAccount.GetDescription()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(serviceAccount.GetId())
	return nil
}
