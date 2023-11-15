// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccNodeResource(t *testing.T) {
	// Note that we deliberately add extra whitespace here to test normalization of JSON strings
	// entityState := []byte(`[
	// 	{"world":{"id":"world"}}
	// ]`)
	var entityState string = ""
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			if entityState == "" {
				t.Log("GET returning 404")
				w.WriteHeader(404)
			} else {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(entityState))
				if err != nil {
					t.Errorf("error writing body: %s", err)
				}
				t.Logf("GET returning %s", entityState)
			}
		case "PUT":
			buf := new(strings.Builder)
			_, err := io.Copy(buf, r.Body)
			entityState = buf.String()
			t.Logf("PUT body %s", entityState)
			if err != nil {
				t.Errorf("error reading body: %s", err)
			}
		case "DELETE":
			entityState = ""
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	defer testServer.Close()

	base := baseURL(testServer.URL)
	path := "/bosk/path/to/object"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNodeResourceConfig(base, path, []map[string]map[string]string{
					{"world": {"id": "world"}},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "path", path),
					resource.TestCheckResourceAttr("bosk_node.test", "value_json", "[{\"world\":{\"id\":\"world\"}}]"),
				),
			},
			// ImportState testing
			{
				ResourceName:                         "bosk_node.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "path",
				ImportStateId:                        path,
			},
			// // Update and Read testing
			{
				Config: testAccNodeResourceConfig(base, path, []map[string]map[string]string{
					{"someone": {"id": "someone"}},
					{"anyone": {"id": "anyone"}},
				}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("bosk_node.test", "value_json", `[{"someone":{"id":"someone"}},{"anyone":{"id":"anyone"}}]`),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccNodeResourceConfig(base, path string, value any) string {
	json, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`
		provider "bosk" {
			base_url              = "%s"
			basic_auth_var_suffix = "NO_AUTH"
		}
		resource "bosk_node" "test" {
			path       = "%s"
			value_json = %s
		}
	`, base, path, strconv.Quote(string(json)))
}
