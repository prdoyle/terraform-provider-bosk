// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNodeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccNodeDataSourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.bosk_node.test", "url", "http://localhost:1740/bosk/targets"),
					resource.TestCheckResourceAttr("data.bosk_node.test", "value_json", "[{\"world\":{\"id\":\"world\"}}]"),
				),
			},
		},
	})
}

const testAccNodeDataSourceConfig = `
data "bosk_node" "test" {
  url = "http://localhost:1740/bosk/targets"
  value_json = "[]"
}
`
