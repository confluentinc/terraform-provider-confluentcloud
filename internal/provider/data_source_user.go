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
	"net/http"
)

func userAccountDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: userAccountDataSourceRead,
		Schema: map[string]*schema.Schema{
			paramId: {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				// A user should provide a value for either "id", "display_name", or email attribute, not more than one
				ExactlyOneOf: []string{paramId, paramDisplayName, paramEmail},
				Description:  "The ID of the User Account (e.g., `u-l793v1`).",
			},
			paramApiVersion: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "API Version defines the schema version of this representation of a User Account.",
			},
			paramKind: {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "Kind defines the object User Account represents.",
			},
			paramDisplayName: {
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
				// A user should provide a value for either "id", "display_name", or email attribute, not more than one
				ExactlyOneOf: []string{paramId, paramDisplayName, paramEmail},
				Description:  "A full display name of the user.",
			},
			paramEmail: {
				Type:        schema.TypeString,
				Computed:    true,
				Optional:    true,
				// A user should provide a value for either "id", "display_name", or email attribute, not more than one
				ExactlyOneOf: []string{paramId, paramDisplayName, paramEmail},
				Description: "Email address of the user.",
			},
		},
	}
}

func userAccountDataSourceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	// ExactlyOneOf specified in the schema ensures one of paramId or paramDisplayName is specified.
	// The next step is to figure out which one exactly is set.

	userAccountId := d.Get(paramId).(string)
	displayName := d.Get(paramDisplayName).(string)
	email := d.Get(paramEmail).(string)

	if userAccountId != "" {
		return userAccountDataSourceReadUsingId(ctx, d, meta, userAccountId)
	} else if displayName != "" {
		return userAccountDataSourceReadUsingDisplayName(ctx, d, meta, displayName)
	} else if email != "" {
		return userAccountDataSourceReadUsingEmail(ctx, d, meta, email)
	} else {
		return diag.Errorf("error creating confluentcloud_user data source: one of \"%s\", \"%s\", or \"%s\" must be specified but they're all empty", paramId, paramDisplayName, paramEmail)
	}
}

func userAccountDataSourceReadUsingDisplayName(ctx context.Context, d *schema.ResourceData, meta interface{}, displayName string) diag.Diagnostics {
	log.Printf("[INFO] User account read using \"%s\"=%s", paramDisplayName, displayName)

	c := meta.(*Client)
	userAccountList, resp, err := c.iamClient.UsersIamV2Api.ListIamV2Users(c.iamApiContext(ctx)).Execute()
	if err != nil {
		log.Printf("[ERROR] User accounts get failed %v, %s", resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if orgHasMultipleUAsWithTargetDisplayName(userAccountList, displayName) {
		return diag.Errorf("There are multiple user accounts with display_name=%s", displayName)
	}

	for _, userAccount := range userAccountList.GetData() {
		if userAccount.GetFullName() == displayName {
			return setUserAccountDataSourceAttributes(d, userAccount)
		}
	}

	return diag.Errorf("The user account with display_name=%s was not found", displayName)
}

func userAccountDataSourceReadUsingEmail(ctx context.Context, d *schema.ResourceData, meta interface{}, email string) diag.Diagnostics {
	log.Printf("[INFO] User account read using \"%s\"=%s", paramEmail, email)

	c := meta.(*Client)
	userAccountList, resp, err := c.iamClient.UsersIamV2Api.ListIamV2Users(c.iamApiContext(ctx)).Execute()
	if err != nil {
		log.Printf("[ERROR] User accounts get failed %v, %s", resp, err)
		return createDiagnosticsWithDetails(err)
	}
	if orgHasMultipleUAsWithTargetEmail(userAccountList, email) {
		return diag.Errorf("There are multiple user accounts with email=%s", email)
	}

	for _, userAccount := range userAccountList.GetData() {
		if userAccount.GetEmail() == email {
			return setUserAccountDataSourceAttributes(d, userAccount)
		}
	}

	return diag.Errorf("The user account with email=%s was not found", email)
}

func executeUserAccountRead(ctx context.Context, c *Client, userAccountId string) (v2.IamV2User, *http.Response, error) {
	req := c.iamClient.UsersIamV2Api.GetIamV2User(c.iamApiContext(ctx), userAccountId)
	return req.Execute()
}

func userAccountDataSourceReadUsingId(ctx context.Context, d *schema.ResourceData, meta interface{}, userAccountId string) diag.Diagnostics {
	log.Printf("[INFO] User account read using \"%s\"=%s", paramId, userAccountId)

	c := meta.(*Client)
	userAccount, resp, err := executeUserAccountRead(c.iamApiContext(ctx), c, userAccountId)
	if err != nil {
		log.Printf("[ERROR] User account get failed for id %s, %v, %s", userAccountId, resp, err)
		return createDiagnosticsWithDetails(err)
	}
	return setUserAccountDataSourceAttributes(d, userAccount)
}

func setUserAccountDataSourceAttributes(d *schema.ResourceData, userAccount v2.IamV2User) diag.Diagnostics {
	if err := d.Set(paramApiVersion, userAccount.GetApiVersion()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramKind, userAccount.GetKind()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramDisplayName, userAccount.GetFullName()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	if err := d.Set(paramEmail, userAccount.GetEmail()); err != nil {
		return createDiagnosticsWithDetails(err)
	}
	d.SetId(userAccount.GetId())
	return nil
}

func orgHasMultipleUAsWithTargetDisplayName(userAccountList v2.IamV2UserList, displayName string) bool {
	var numberOfUserAccountsWithTargetDisplayName = 0
	for _, userAccount := range userAccountList.GetData() {
		if userAccount.GetFullName() == displayName {
			numberOfUserAccountsWithTargetDisplayName += 1
		}
	}
	return numberOfUserAccountsWithTargetDisplayName > 1
}

func orgHasMultipleUAsWithTargetEmail(userAccountList v2.IamV2UserList, email string) bool {
	var numberOfUserAccountsWithTargetEmail = 0
	for _, userAccount := range userAccountList.GetData() {
		if userAccount.GetEmail() == email {
			numberOfUserAccountsWithTargetEmail += 1
		}
	}
	return numberOfUserAccountsWithTargetEmail > 1
}
