// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNodeDataSource(t *testing.T) {
	// Thank you https://github.com/hashicorp/terraform-provider-http/blob/main/internal/provider/data_source_http_test.go
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

	base := testServer.URL
	path := "/bosk/path/to/object"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: fmt.Sprintf(`
				 	provider "bosk" {
						basic_auth_var_suffix = "NO_AUTH"
					}
					data "bosk_node" "test" {
						url        = "%s%s"
						value_json = jsonencode([])
					}
				`, base, path),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.bosk_node.test", "url", base+path),
					resource.TestCheckResourceAttr("data.bosk_node.test", "value_json", "[{\"world\":{\"id\":\"world\"}}]"),
				),
			},
		},
	})
}
