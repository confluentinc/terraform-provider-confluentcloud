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
	v2 "github.com/confluentinc/ccloud-sdk-go-v2/iam/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"log"
)

func serviceAccountDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: serviceAccountDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				// A user should provide a value for either "id" or "display_name" attribute, not both
				ExactlyOneOf: []string{paramId, paramDisplayName},
				Description:  "The ID of the Service Account (e.g., `sa-abc123`).",
			},
			paramApiVersion: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "API Version defines the schema version of this representation of a Service Account.",
			},
			paramKind: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Kind defines the object Service Account represents.",
			},
			paramDisplayName: {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				// A user should provide a value for either "id" or "display_name" attribute, not both
				ExactlyOneOf: []string{paramId, paramDisplayName},
				Description:  "A human-readable name for the Service Account.",
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
	// ExactlyOneOf specified in the schema ensures one of paramId or paramDisplayName is specified.
	// The next step is to figure out which one exactly is set.

	serviceAccountId := d.Get(paramId).(string)
	displayName := d.Get(paramDisplayName).(string)

	if serviceAccountId != "" {
		return serviceAccountDataSourceReadUsingId(ctx, d, meta, serviceAccountId)
	} else if displayName != "" {
		return serviceAccountDataSourceReadUsingDisplayName(ctx, d, meta, displayName)
	} else {
		return diag.Errorf("error creating confluentcloud_service_account data source: one of \"%s\" or \"%s\" must be specified but they're both empty", paramId, paramDisplayName)
	}
}

func serviceAccountDataSourceReadUsingDisplayName(ctx context.Context, d *schema.ResourceData, meta interface{}, displayName string) diag.Diagnostics {
	log.Printf("[INFO] Service account read using \"%s\"=%s", paramDisplayName, displayName)

	c := meta.(*Client)
	serviceAccountList, resp, err := c.iamClient.ServiceAccountsIamV2Api.ListIamV2ServiceAccounts(c.iamApiContext(ctx)).Execute()
	if err != nil {
		log.Printf("[ERROR] Service accounts get failed %v, %s", resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if orgHasMultipleSAsWithTargetDisplayName(serviceAccountList, displayName) {
		return diag.Errorf("There are multiple service accounts with display_name=%s", displayName)
	}

	for _, serviceAccount := range serviceAccountList.GetData() {
		if serviceAccount.GetDisplayName() == displayName {
			return setServiceAccountDataSourceAttributes(d, serviceAccount)
		}
	}

	return diag.Errorf("The service account with display_name=%s was not found", displayName)
}

func serviceAccountDataSourceReadUsingId(ctx context.Context, d *schema.ResourceData, meta interface{}, serviceAccountId string) diag.Diagnostics {
	log.Printf("[INFO] Service account read using \"%s\"=%s", paramId, serviceAccountId)

	c := meta.(*Client)
	serviceAccount, resp, err := executeServiceAccountRead(c.iamApiContext(ctx), c, serviceAccountId)
	if err != nil {
		log.Printf("[ERROR] Service account get failed for id %s, %v, %s", serviceAccountId, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	return setServiceAccountDataSourceAttributes(d, serviceAccount)
}

func setServiceAccountDataSourceAttributes(d *schema.ResourceData, serviceAccount v2.IamV2ServiceAccount) diag.Diagnostics {
	if err := d.Set(paramApiVersion, serviceAccount.GetApiVersion()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramKind, serviceAccount.GetKind()); err != nil {
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

func orgHasMultipleSAsWithTargetDisplayName(serviceAccountList v2.IamV2ServiceAccountList, displayName string) bool {
	var numberOfServiceAccountsWithTargetDisplayName = 0
	for _, serviceAccount := range serviceAccountList.GetData() {
		if serviceAccount.GetDisplayName() == displayName {
			numberOfServiceAccountsWithTargetDisplayName += 1
		}
	}
	return numberOfServiceAccountsWithTargetDisplayName > 1
}
