// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure BoskProvider satisfies various provider interfaces.
var _ provider.Provider = &BoskProvider{}

// BoskProvider defines the provider implementation.
type BoskProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// BoskProviderModel describes the provider data model.
type BoskProviderModel struct {
	BasicAuthVarSuffix types.String `tfsdk:"basic_auth_var_suffix"`
}

func (p *BoskProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "bosk"
	resp.Version = p.version
}

func (p *BoskProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"basic_auth_var_suffix": schema.StringAttribute{
				MarkdownDescription: "Selects the environment variables to use for HTTP basic authentication; namely TF_BOSK_USERNAME_xxx and TF_BOSK_PASSWORD_xxx. If you don't want to use basic auth, specify NO_AUTH.",
				Required:            true,
			},
		},
	}
}

func (p *BoskProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data BoskProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var suffix = data.BasicAuthVarSuffix.ValueString()
	var usernameVar = "TF_BOSK_USERNAME_" + suffix
	var passwordVar = "TF_BOSK_PASSWORD_" + suffix
	username, usernameExists := os.LookupEnv(usernameVar)
	password, passwordExists := os.LookupEnv(passwordVar)
	var client *BoskClient

	if suffix == "NO_AUTH" {
		client = NewBoskClientWithoutAuth(http.DefaultClient)
		if usernameExists {
			resp.Diagnostics.AddWarning(
				"NO_AUTH suffix overrides username environment variable",
				fmt.Sprintf("Based on basic_auth_var_suffix of \"%v\", ignoring environment variable \"TF_BOSK_USERNAME_%v\"", suffix, suffix),
			)
		}
		if passwordExists {
			resp.Diagnostics.AddWarning(
				"NO_AUTH suffix overrides password environment variable",
				fmt.Sprintf("Based on basic_auth_var_suffix of \"%v\", ignoring environment variable \"TF_BOSK_PASSWORD_%v\"", suffix, suffix),
			)
		}
	} else if usernameExists && passwordExists {
		client = NewBoskClient(http.DefaultClient, username, password)
	} else {
		resp.Diagnostics.AddError(
			"Missing environment variables for authentication",
			fmt.Sprintf("Based on basic_auth_var_suffix of \"%v\", expected to find environment variables \"TF_BOSK_USERNAME_%v\" and \"TF_BOSK_PASSWORD_%v\"", suffix, suffix, suffix),
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *BoskProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNodeResource,
	}
}

func (p *BoskProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewNodeDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &BoskProvider{
			version: version,
		}
	}
}
