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
	org "github.com/confluentinc/ccloud-sdk-go-v2/org/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"log"
	"net/http"
)

func environmentResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: environmentCreate,
		ReadContext:   environmentRead,
		UpdateContext: environmentUpdate,
		DeleteContext: environmentDelete,
		Importer: &schema.ResourceImporter{
			StateContext: environmentImport,
		},
		Schema: map[string]*schema.Schema{
			paramDisplayName: {
				Type:         schema.TypeString,
				Description:  "A human-readable name for the Environment.",
				ValidateFunc: validation.StringIsNotEmpty,
				Required:     true,
			},
		},
	}
}

func environmentUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := extractDisplayName(d)

	env := org.NewV2Environment()
	if displayName != "" {
		env.SetDisplayName(displayName)
	}

	env.SetId(d.Id())

	req := c.orgClient.EnvironmentsV2Api.UpdateV2Environment(c.orgApiContext(ctx), d.Id()).V2Environment(*env)
	_, _, err := req.Execute()

	if err != nil {
		diag.FromErr(err)
	}

	return nil
}

func environmentCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	displayName := extractDisplayName(d)
	env := org.NewV2Environment()
	env.SetDisplayName(displayName)

	createdEnv, resp, err := executeEnvironmentCreate(c.orgApiContext(ctx), c, env)
	if err != nil {
		log.Printf("[ERROR] Environment create failed %v, %v, %s", env, resp, err)
		return diag.FromErr(err)
	}
	d.SetId(createdEnv.GetId())
	log.Printf("[DEBUG] Created environment %s", createdEnv.GetId())

	return nil
}

func executeEnvironmentCreate(ctx context.Context, c *Client, environment *org.V2Environment) (org.V2Environment, *http.Response, error) {
	req := c.orgClient.EnvironmentsV2Api.CreateV2Environment(c.orgApiContext(ctx)).V2Environment(*environment)

	return req.Execute()
}

func environmentDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	req := c.orgClient.EnvironmentsV2Api.DeleteV2Environment(c.orgApiContext(ctx), d.Id())
	_, err := req.Execute()

	if err != nil {
		return diag.FromErr(fmt.Errorf("error deleting environment (%s), err: %s", d.Id(), err))
	}

	return nil
}

func environmentImport(_ context.Context, d *schema.ResourceData, _ interface{}) ([]*schema.ResourceData, error) {
	return []*schema.ResourceData{d}, nil
}

func executeEnvironmentRead(ctx context.Context, c *Client, environmentId string) (org.V2Environment, *http.Response, error) {
	req := c.orgClient.EnvironmentsV2Api.GetV2Environment(c.orgApiContext(ctx), environmentId)
	return req.Execute()
}

func environmentRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Environment read for %s", d.Id())
	c := meta.(*Client)
	environment, resp, err := executeEnvironmentRead(c.orgApiContext(ctx), c, d.Id())
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}
	if err != nil {
		log.Printf("[ERROR] Environment get failed for id %s, %v, %s", d.Id(), resp, err)
	}
	if err == nil {
		err = d.Set(paramDisplayName, environment.GetDisplayName())
	}
	return diag.FromErr(err)
}
