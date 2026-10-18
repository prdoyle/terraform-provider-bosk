package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/prdoyle/terraform-provider-bosk/internal/testserver"
)

func testAccNodeDataSourceConfig(server testserver.Server, path string) string {
	return testAccProviderConfig(server) + fmt.Sprintf(`
data "bosk_node" "test" {
  path = %q
}
`, path)
}

func TestAccNodeDataSource(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeDataSourceConfig(fake, "/versioned/v1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.bosk_node.test", "path", "/versioned/v1"),
					resource.TestCheckResourceAttr("data.bosk_node.test", "value", `{"id":"v1","revision":"rev1","value":"one"}`),
					resource.TestCheckResourceAttr("data.bosk_node.test", "etag", `"rev1"`),
				),
			},
		},
	})
}

func TestAccNodeDataSource_notFound(t *testing.T) {
	fake := testserver.NewFake(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccNodeDataSourceConfig(fake, "/targets/missing"),
				ExpectError: regexp.MustCompile(`not found`),
			},
		},
	})
}
