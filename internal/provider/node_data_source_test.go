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
		_, err := w.Write([]byte("[{\"world\":{\"id\":\"world\"}}]"))
		if err != nil {
			t.Errorf("error writing body: %s", err)
		}
	}))
	defer testServer.Close()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: fmt.Sprintf(`
					data "bosk_node" "test" {
						url        = "%s"
						value_json = "[]"
					}
				`, testServer.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.bosk_node.test", "url", testServer.URL),
					resource.TestCheckResourceAttr("data.bosk_node.test", "value_json", "[{\"world\":{\"id\":\"world\"}}]"),
				),
			},
		},
	})
}
