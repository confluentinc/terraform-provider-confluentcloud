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
	"github.com/docker/go-connections/nat"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/walkerus/go-wiremock"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

const (
	uaApiVersion             = "iam/v2"
	uaDataSourceScenarioName = "confluentcloud_user Data Source Lifecycle"
	uaId                     = "u-l793v1"
	uaDisplayName            = "test_user_full_name"
	uaEmail                  = "user@example.com"
	uaKind                   = "User"
	uaResourceLabel          = "test_user"
)

func TestAccDataSourceUserAccount(t *testing.T) {
	containerPort := "8080"
	containerPortTcp := fmt.Sprintf("%s/tcp", containerPort)
	ctx := context.Background()
	listeningPort := wait.ForListeningPort(nat.Port(containerPortTcp))
	req := testcontainers.ContainerRequest{
		Image:        "rodolpheche/wiremock",
		ExposedPorts: []string{containerPortTcp},
		WaitingFor:   listeningPort,
	}
	wiremockContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	require.NoError(t, err)

	// nolint:errcheck
	defer wiremockContainer.Terminate(ctx)

	host, err := wiremockContainer.Host(ctx)
	require.NoError(t, err)

	wiremockHttpMappedPort, err := wiremockContainer.MappedPort(ctx, nat.Port(containerPort))
	require.NoError(t, err)

	mockServerUrl := fmt.Sprintf("http://%s:%s", host, wiremockHttpMappedPort.Port())
	wiremockClient := wiremock.NewClient(mockServerUrl)
	// nolint:errcheck
	defer wiremockClient.Reset()

	// nolint:errcheck
	defer wiremockClient.ResetAllScenarios()

	readCreatedUaResponse, _ := ioutil.ReadFile("../testdata/user_account/read_created_ua.json")
	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo("/iam/v2/users/u-l793v1")).
		InScenario(uaDataSourceScenarioName).
		WhenScenarioStateIs(wiremock.ScenarioStateStarted).
		WillReturn(
			string(readCreatedUaResponse),
			contentTypeJSONHeader,
			http.StatusOK,
		))

	readUserAccountsResponse, _ := ioutil.ReadFile("../testdata/user_account/read_uas.json")
	_ = wiremockClient.StubFor(wiremock.Get(wiremock.URLPathEqualTo("/iam/v2/users")).
		InScenario(envScenarioDataSourceName).
		WhenScenarioStateIs(wiremock.ScenarioStateStarted).
		WillReturn(
			string(readUserAccountsResponse),
			contentTypeJSONHeader,
			http.StatusOK,
		))

	fullUserAccountDataSourceLabel := fmt.Sprintf("data.confluentcloud_user.%s", uaResourceLabel)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		// https://www.terraform.io/docs/extend/testing/acceptance-tests/teststep.html
		// https://www.terraform.io/docs/extend/best-practices/testing.html#built-in-patterns
		Steps: []resource.TestStep{
			{
				Config: testAccCheckDataSourceUserAccountConfigWithIdSet(mockServerUrl, uaResourceLabel, uaId),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserAccountExists(fullUserAccountDataSourceLabel),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramId, uaId),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramApiVersion, uaApiVersion),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramKind, uaKind),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramDisplayName, uaDisplayName),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramEmail, uaEmail),
				),
			},
			{
				Config: testAccCheckDataSourceUserAccountConfigWithFullNameSet(mockServerUrl, uaResourceLabel, uaDisplayName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserAccountExists(fullUserAccountDataSourceLabel),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramId, uaId),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramApiVersion, uaApiVersion),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramKind, uaKind),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramDisplayName, uaDisplayName),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramEmail, uaEmail),
				),
			},
			{
				Config: testAccCheckDataSourceUserAccountConfigWithEmailSet(mockServerUrl, uaResourceLabel, uaEmail),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserAccountExists(fullUserAccountDataSourceLabel),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramId, uaId),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramApiVersion, uaApiVersion),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramKind, uaKind),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramDisplayName, uaDisplayName),
					resource.TestCheckResourceAttr(fullUserAccountDataSourceLabel, paramEmail, uaEmail),
				),
			},
		},
	})
}

func testAccCheckDataSourceUserAccountConfigWithIdSet(mockServerUrl, uaResourceLabel, uaId string) string {
	return fmt.Sprintf(`
	provider "confluentcloud" {
		endpoint = "%s"
	}
	data "confluentcloud_user" "%s" {
		id = "%s"
	}
	`, mockServerUrl, uaResourceLabel, uaId)
}

func testAccCheckDataSourceUserAccountConfigWithFullNameSet(mockServerUrl, uaResourceLabel, fullName string) string {
	return fmt.Sprintf(`
	provider "confluentcloud" {
		endpoint = "%s"
	}
	data "confluentcloud_user" "%s" {
		display_name = "%s"
	}
	`, mockServerUrl, uaResourceLabel, fullName)
}

func testAccCheckDataSourceUserAccountConfigWithEmailSet(mockServerUrl, uaResourceLabel, email string) string {
	return fmt.Sprintf(`
	provider "confluentcloud" {
		endpoint = "%s"
	}
	data "confluentcloud_user" "%s" {
		email = "%s"
	}
	`, mockServerUrl, uaResourceLabel, email)
}

func testAccCheckUserAccountConfig(mockServerUrl, uaResourceLabel, uaDisplayName, uaEmail string) string {
	return fmt.Sprintf(`
	provider "confluentcloud" {
		endpoint = "%s"
	}
	resource "confluentcloud_user" "%s" {
		display_name = "%s"
		email = "%s"
	}
	`, mockServerUrl, uaResourceLabel, uaDisplayName, uaEmail)
}

func testAccCheckUserAccountExists(n string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]

		if !ok {
			return fmt.Errorf("%s user account has not been found", n)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("ID has not been set for %s user account", n)
		}

		return nil
	}
}
