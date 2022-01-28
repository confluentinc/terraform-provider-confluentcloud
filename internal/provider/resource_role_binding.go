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
	mds "github.com/confluentinc/ccloud-sdk-go-v2/mds/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"log"
	"net/http"
	"regexp"
)

const (
	paramRoleName   = "role_name"
	paramCrnPattern = "crn_pattern"
)

func roleBindingResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: roleBindingCreate,
		ReadContext:   roleBindingRead,
		DeleteContext: roleBindingDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			paramPrincipal: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "The principal User to bind the role to.",
				ValidateFunc: validation.StringMatch(regexp.MustCompile("^User:"), "the Principal must be of the form 'User:'"),
			},
			paramRoleName: {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The name of the role to bind to the principal.",
			},
			paramCrnPattern: {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				Description:  "A CRN that specifies the scope and resource patterns necessary for the role to bind.",
				ValidateFunc: validation.StringMatch(regexp.MustCompile("^crn://"), "the CRN must be of the form 'crn://'"),
			},
		},
	}
}

func roleBindingCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	c := meta.(*Client)

	principal := d.Get(paramPrincipal).(string)
	roleName := d.Get(paramRoleName).(string)
	crnPattern := d.Get(paramCrnPattern).(string)

	log.Printf("[INFO] Role binding create for %s", roleName)

	roleBinding := mds.NewIamV2RoleBinding()
	roleBinding.SetPrincipal(principal)
	roleBinding.SetRoleName(roleName)
	roleBinding.SetCrnPattern(crnPattern)

	createdRoleBinding, resp, err := executeRoleBindingCreate(c.mdsApiContext(ctx), c, roleBinding)
	if err != nil {
		log.Printf("[ERROR] role binding create failed %v, %v, %s", roleBinding, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(createdRoleBinding.GetId())
	log.Printf("[DEBUG] Created role binding id: %s", createdRoleBinding.GetId())

	return roleBindingRead(ctx, d, meta)
}

func executeRoleBindingCreate(ctx context.Context, c *Client, roleBinding *mds.IamV2RoleBinding) (mds.IamV2RoleBinding, *http.Response, error) {
	req := c.mdsClient.RoleBindingsIamV2Api.CreateIamV2RoleBinding(c.mdsApiContext(ctx)).IamV2RoleBinding(*roleBinding)
	return req.Execute()
}

func roleBindingDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Role binding delete for %s", d.Id())
	c := meta.(*Client)

	req := c.mdsClient.RoleBindingsIamV2Api.DeleteIamV2RoleBinding(c.mdsApiContext(ctx), d.Id())
	_, err := req.Execute()

	if err != nil {
		return diag.Errorf("error deleting role binding (%s), err: %s", d.Id(), err)
	}

	return nil
}

func roleBindingRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf("[INFO] Role binding read for %s", d.Id())
	c := meta.(*Client)
	roleBinding, resp, err := executeRoleBindingRead(c.mdsApiContext(ctx), c, d.Id())
	if err != nil {
		log.Printf("[WARN] Role binding get failed for id %s, %v, %s", d.Id(), resp, err)

		// https://learn.hashicorp.com/tutorials/terraform/provider-setup
		isResourceNotFound := HasStatusForbidden(resp) && !HasStatusForbiddenDueToInvalidAPIKey(resp)
		if isResourceNotFound {
			log.Printf("[WARN] Role binding with id=%s is not found", d.Id())
			// If the resource isn't available, Terraform destroys the resource in state.
			d.SetId("")
			return nil
		}

		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramPrincipal, roleBinding.GetPrincipal()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramRoleName, roleBinding.GetRoleName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramCrnPattern, roleBinding.GetCrnPattern()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	return nil
}
func executeRoleBindingRead(ctx context.Context, c *Client, roleBindingId string) (mds.IamV2RoleBinding, *http.Response, error) {
	req := c.mdsClient.RoleBindingsIamV2Api.GetIamV2RoleBinding(c.mdsApiContext(ctx), roleBindingId)
	return req.Execute()
}
