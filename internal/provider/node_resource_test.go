// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNodeResource(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Note that we deliberately add extra whitespace here to test normalization of JSON strings
		_, err := w.Write([]byte(`
			[
				{"world":{"id":"world"}}
			]
		`))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	fmt.Println("Testing testAccNodeResourceConfig: ", testAccNodeResourceConfig(testServer.URL, []map[string]map[string]string{
		{"world": {"id": "world"}},
	}))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNodeResourceConfig(testServer.URL, []map[string]map[string]string{
					{"world": {"id": "world"}},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "url", testServer.URL),
					resource.TestCheckResourceAttr("bosk_node.test", "value_json", "[{\"world\":{\"id\":\"world\"}}]"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "bosk_node.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// // Update and Read testing
			// {
			// 	Config: testAccNodeResourceConfig("two"),
			// 	Check: resource.ComposeAggregateTestCheckFunc(
			// 		resource.TestCheckResourceAttr("bosk_node.test", "configurable_attribute", "two"),
			// 	),
			// },
			// // Delete testing automatically occurs in TestCase
		},
	})
}

func testAccNodeResourceConfig(url string, value any) string {
	json, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`
		resource "bosk_node" "test" {
			url        = "%s"
			value_json = %s
		}
	`, url, strconv.Quote(string(json)))
}
