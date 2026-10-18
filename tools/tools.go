// Copyright (c) 2026 Patrick Doyle
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

// Format Terraform code for use in documentation.
//go:generate terraform fmt -recursive ../examples/

// Generate documentation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. -provider-name bosk
