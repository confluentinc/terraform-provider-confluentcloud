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
	iam "github.com/confluentinc/ccloud-sdk-go-v2/iam/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"log"
	"net/http"
)

func serviceAccountResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: serviceAccountCreate,
		ReadContext:   serviceAccountRead,
		UpdateContext: serviceAccountUpdate,
		DeleteContext: serviceAccountDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			paramDisplayName: {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "A human-readable name for the Service Account.",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			paramDescription: {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A free-form description of the Service Account.",
			},
		},
	}
}

func serviceAccountUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	if d.HasChangeExcept(paramDescription) {
		return diag.Errorf(fmt.Sprintf("only %s field can be updated for a service account", paramDescription))
	}

	updatedServiceAccount := iam.NewIamV2ServiceAccountUpdate()
	updatedDescription := d.Get(paramDescription).(string)
	updatedServiceAccount.SetDescription(updatedDescription)

	c := meta.(*Client)
	_, _, err := c.iamClient.ServiceAccountsIamV2Api.UpdateIamV2ServiceAccount(c.iamApiContext(ctx), d.Id()).IamV2ServiceAccountUpdate(*updatedServiceAccount).Execute()

	if err != nil {
		return createDiagnosticsWithDetails(err)
	}

	return serviceAccountRead(ctx, d, meta)
}

func serviceAccountCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := d.Get(paramDisplayName).(string)
	description := d.Get(paramDescription).(string)

	serviceAccount := iam.NewIamV2ServiceAccount()
	serviceAccount.SetDisplayName(displayName)
	serviceAccount.SetDescription(description)

	createdServiceAccount, resp, err := executeServiceAccountCreate(c.iamApiContext(ctx), c, serviceAccount)
	if err != nil {
		log.Printf("[ERROR] service account create failed %v, %v, %s", serviceAccount, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(createdServiceAccount.GetId())
	log.Printf("[DEBUG] Created service account %s", createdServiceAccount.GetId())

	return serviceAccountRead(ctx, d, meta)
}

func executeServiceAccountCreate(ctx context.Context, c *Client, serviceAccount *iam.IamV2ServiceAccount) (iam.IamV2ServiceAccount, *http.Response, error) {
	req := c.iamClient.ServiceAccountsIamV2Api.CreateIamV2ServiceAccount(c.iamApiContext(ctx)).IamV2ServiceAccount(*serviceAccount)
	return req.Execute()
}

func serviceAccountDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	req := c.iamClient.ServiceAccountsIamV2Api.DeleteIamV2ServiceAccount(c.iamApiContext(ctx), d.Id())
	_, err := req.Execute()

	if err != nil {
		return diag.Errorf("error deleting service account (%s), err: %s", d.Id(), err)
	}

	return nil
}

func executeServiceAccountRead(ctx context.Context, c *Client, serviceAccountId string) (iam.IamV2ServiceAccount, *http.Response, error) {
	req := c.iamClient.ServiceAccountsIamV2Api.GetIamV2ServiceAccount(c.iamApiContext(ctx), serviceAccountId)
	return req.Execute()
}

func serviceAccountRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Service account read for %s", d.Id())
	c := meta.(*Client)
	serviceAccount, resp, err := executeServiceAccountRead(c.iamApiContext(ctx), c, d.Id())
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if err != nil {
		log.Printf("[ERROR] Service account get failed for id %s, %v, %s", d.Id(), resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDisplayName, serviceAccount.GetDisplayName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDescription, serviceAccount.GetDescription()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	return nil
}
