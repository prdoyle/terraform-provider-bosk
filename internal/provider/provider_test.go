// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"bosk": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}

func splitURL(url string) (base, path string) {
	parts := strings.SplitN(url, "/", 4)
	protocol := parts[0]
	host := parts[2]
	if len(parts) == 4 {
		path = "/" + parts[3]
	} else {
		path = "/"
	}
	base = protocol + "//" + host
	//fmt.Printf("HEY HEY base: \"%v\", path: \"%v\"", base, path)
	return base, path
}
