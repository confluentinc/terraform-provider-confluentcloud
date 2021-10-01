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
			StateContext: serviceAccountImport,
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
	if d.HasChange(paramDisplayName) {
		return diag.FromErr(fmt.Errorf("display_name field cannot be updated for a service account"))
	}

	c := meta.(*Client)

	description := extractDescription(d)

	updateReq := iam.NewV2ServiceAccountUpdate()

	// TODO: need a way to explicitly set values to empty strings (*string)
	if description != "" {
		updateReq.SetDescription(description)
	}

	req := c.iamClient.ServiceAccountsV2Api.UpdateV2ServiceAccount(c.iamApiContext(ctx), d.Id()).V2ServiceAccountUpdate(*updateReq)

	_, _, err := req.Execute()

	if err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func serviceAccountCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := extractDisplayName(d)
	description := extractDescription(d)

	serviceAccount := iam.NewV2ServiceAccount()
	serviceAccount.SetDisplayName(displayName)
	serviceAccount.SetDescription(description)

	createdServiceAccount, resp, err := executeServiceAccountCreate(c.iamApiContext(ctx), c, serviceAccount)
	if err != nil {
		log.Printf("[ERROR] service account create failed %v, %v, %s", serviceAccount, resp, err)
		return diag.FromErr(err)
	}
	d.SetId(createdServiceAccount.GetId())
	log.Printf("[DEBUG] Created service account %s", createdServiceAccount.GetId())

	return nil
}

func executeServiceAccountCreate(ctx context.Context, c *Client, serviceAccount *iam.V2ServiceAccount) (iam.V2ServiceAccount, *http.Response, error) {
	req := c.iamClient.ServiceAccountsV2Api.CreateV2ServiceAccount(c.iamApiContext(ctx)).V2ServiceAccount(*serviceAccount)

	return req.Execute()
}

func serviceAccountDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	req := c.iamClient.ServiceAccountsV2Api.DeleteV2ServiceAccount(c.iamApiContext(ctx), d.Id())
	_, err := req.Execute()

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting service account (%s), err: %s", d.Id(), err))
	}

	return nil
}

func serviceAccountImport(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	return []*schema.ResourceData{d}, nil
}

func executeServiceAccountRead(ctx context.Context, c *Client, serviceAccountId string) (iam.V2ServiceAccount, *http.Response, error) {
	req := c.iamClient.ServiceAccountsV2Api.GetV2ServiceAccount(c.iamApiContext(ctx), serviceAccountId)
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
	}
	if err == nil {
		err = d.Set(paramDisplayName, serviceAccount.GetDisplayName())
	}
	if err == nil {
		err = d.Set(paramDescription, serviceAccount.GetDescription())
	}
	return diag.FromErr(err)
}
