package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/prdoyle/terraform-provider-bosk/internal/testserver"
)

func testAccProviderConfig(server testserver.Server) string {
	return testAccProviderConfigWithTimeout(server, "5s")
}

func testAccProviderConfigWithTimeout(server testserver.Server, timeout string) string {
	auth := ""
	switch {
	case server.Token() != "":
		auth = fmt.Sprintf("  token = %q\n", server.Token())
	case server.Username() != "":
		auth = fmt.Sprintf("  username = %q\n  password = %q\n", server.Username(), server.Password())
	}
	return fmt.Sprintf(`
provider "bosk" {
  endpoint        = %q
  request_timeout = %q
%s}
`, server.Endpoint(), timeout, auth)
}

func testAccNodeConfig(server testserver.Server, path, valueExpression string) string {
	return testAccProviderConfig(server) + fmt.Sprintf(`
resource "bosk_node" "test" {
  path  = %q
  value = %s
}
`, path, valueExpression)
}

func TestAccNode_basic(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/new", `jsonencode({ id = "new", name = "New" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/new"),
					resource.TestCheckResourceAttr("bosk_node.test", "value", `{"id":"new","name":"New"}`),
					resource.TestCheckNoResourceAttr("bosk_node.test", "etag"),
				),
			},
			{
				Config: testAccNodeConfig(fake, "/targets/new", `jsonencode({ id = "new", name = "Updated" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "value", `{"id":"new","name":"Updated"}`),
				),
			},
			{
				ResourceName:                         "bosk_node.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "path",
				ImportStateId:                        "/targets/new",
			},
			{
				ResourceName:    "bosk_node.test",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithResourceIdentity,
			},
		},
	})
}

func TestAccNode_versionedETag(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/versioned/v3", `jsonencode({ id = "v3", revision = "rev1", value = "one" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "value", `{"id":"v3","revision":"rev1","value":"one"}`),
					resource.TestCheckResourceAttr("bosk_node.test", "etag", `"rev1"`),
				),
			},
			{
				Config: testAccNodeConfig(fake, "/versioned/v3", `jsonencode({ id = "v3", revision = "rev2", value = "two" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "value", `{"id":"v3","revision":"rev2","value":"two"}`),
					resource.TestCheckResourceAttr("bosk_node.test", "etag", `"rev2"`),
				),
			},
		},
	})
}

func TestAccNode_staleETagConflict(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/versioned/v4", `jsonencode({ id = "v4", revision = "rev1", value = "one" })`),
			},
			{
				Config: testAccNodeConfig(fake, "/versioned/v4", `jsonencode({ id = "v4", revision = "rev3", value = "three" })`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						mutateOnPlan(func() {
							fake.SetNode("/versioned/v4", `{"id":"v4","revision":"rev2","value":"two"}`)
						}),
					},
				},
				ExpectError: regexp.MustCompile(`changed concurrently`),
			},
			{
				// Leave the resource in a state the post-test destroy can
				// delete: restore the value the provider believes it manages,
				// then apply the change that was blocked above.
				PreConfig: func() {
					fake.SetNode("/versioned/v4", `{"id":"v4","revision":"rev1","value":"one"}`)
				},
				Config: testAccNodeConfig(fake, "/versioned/v4", `jsonencode({ id = "v4", revision = "rev2", value = "two" })`),
			},
		},
	})
}

func TestAccNode_alreadyExists(t *testing.T) {
	fake := testserver.NewFake(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccNodeConfig(fake, "/targets/world", `jsonencode({ id = "world", name = "Other" })`),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

func TestAccNode_disappears(t *testing.T) {
	fake := testserver.NewFake(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/gone", `jsonencode({ id = "gone" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/gone"),
					func(state *terraform.State) error {
						fake.DeleteNode("/targets/gone")
						return nil
					},
				),
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccNodeConfig(fake, "/targets/gone", `jsonencode({ id = "gone" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/gone"),
				),
			},
		},
	})
}

func TestAccNode_pathEncoding(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/c%2Fd", `jsonencode({ id = "c/d", name = "Slashy" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/c%2Fd"),
					func(state *terraform.State) error {
						if _, ok := fake.GetNode("/targets/c%2Fd"); !ok {
							return fmt.Errorf("fake server does not contain the percent-encoded path")
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccNode_jsonSemanticEquality(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/semantic", `jsonencode({ a = 1, b = 2 })`),
			},
			{
				Config:   testAccNodeConfig(fake, "/targets/semantic", `"{ \"b\": 2, \"a\": 1 }"`),
				PlanOnly: true,
			},
		},
	})
}

func TestAccNode_bearerAuth(t *testing.T) {
	fake := testserver.NewFake(t).WithBearer("secret-token")
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/auth", `jsonencode({ id = "auth" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/auth"),
				),
			},
		},
	})
}

func TestAccNode_basicAuth(t *testing.T) {
	fake := testserver.NewFake(t).WithBasic("user", "pass")
	fake.RequireFreshReads()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/basic", `jsonencode({ id = "basic" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/basic"),
				),
			},
		},
	})
}

func TestAccNode_authFailure(t *testing.T) {
	fake := testserver.NewFake(t).WithBearer("secret-token")

	config := fmt.Sprintf(`
provider "bosk" {
  endpoint        = %q
  token           = "wrong-token"
  request_timeout = "5s"
}
resource "bosk_node" "test" {
  path  = "/targets/auth"
  value = jsonencode({ id = "auth" })
}
`, fake.Endpoint())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`bosk API error`),
			},
		},
	})
}

func TestAccNode_apiError(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.FailAlways("PUT", "/bosk/state/targets/fault", 400, "bad value", nil)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccNodeConfig(fake, "/targets/fault", `jsonencode({ id = "fault" })`),
				ExpectError: regexp.MustCompile(`bad value`),
			},
		},
	})
}

func TestAccNode_retryAfter(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.FailNext("PUT", "/bosk/state/targets/retry", 503, "busy", map[string]string{"Retry-After": "0"})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeConfig(fake, "/targets/retry", `jsonencode({ id = "retry" })`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", "/targets/retry"),
				),
			},
		},
	})
}

func TestAccNode_timeout(t *testing.T) {
	fake := testserver.NewFake(t)
	fake.DelayNext("PUT", "/bosk/state/targets/slow", 200*time.Millisecond)

	config := testAccProviderConfigWithTimeout(fake, "20ms") + `
resource "bosk_node" "test" {
  path  = "/targets/slow"
  value = jsonencode({ id = "slow" })
}
`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?i)deadline exceeded|timeout`),
			},
		},
	})
}

type mutateOnPlan func()

func (f mutateOnPlan) CheckPlan(ctx context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	f()
}
