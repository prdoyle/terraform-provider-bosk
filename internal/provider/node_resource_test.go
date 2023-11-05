// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNodeResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNodeResourceConfig("one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "configurable_attribute", "one"),
					resource.TestCheckResourceAttr("bosk_node.test", "defaulted", "example value when not configured"),
					resource.TestCheckResourceAttr("bosk_node.test", "id", "example-id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "bosk_node.test",
				ImportState:       true,
				ImportStateVerify: true,
				// This is not normally necessary, but is here because this
				// example code does not have an actual upstream service.
				// Once the Read method is able to refresh information from
				// the upstream service, this can be removed.
				ImportStateVerifyIgnore: []string{"configurable_attribute", "defaulted"},
			},
			// Update and Read testing
			{
				Config: testAccNodeResourceConfig("two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "configurable_attribute", "two"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccNodeResourceConfig(configurableAttribute string) string {
	return fmt.Sprintf(`
resource "bosk_node" "test" {
  configurable_attribute = %[1]q
}
`, configurableAttribute)
}
