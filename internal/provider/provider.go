package provider

import (
  "context"

  "github.com/hashicorp/terraform-plugin-framework/datasource"
  "github.com/hashicorp/terraform-plugin-framework/path"
  "github.com/hashicorp/terraform-plugin-framework/provider"
  "github.com/hashicorp/terraform-plugin-framework/provider/schema"
  "github.com/hashicorp/terraform-plugin-framework/resource"
  "github.com/hashicorp/terraform-plugin-framework/types"

  "terraform-provider-bosk/internal/client"
)

// Ensure the implementation satisfies the expected interfaces.
var (
    _ provider.Provider = &boskProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
    return func() provider.Provider {
        return &boskProvider{
            version: version,
        }
    }
}

// boskProvider is the provider implementation.
type boskProvider struct {
    // version is set to the provider version on release, "dev" when the
    // provider is built and ran locally, and "test" when running acceptance
    // testing.
    version string
}

// Metadata returns the provider type name.
func (p *boskProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
    resp.TypeName = "bosk"
    resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *boskProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "baseURL": schema.StringAttribute{
	    },
            "username": schema.StringAttribute{
                Optional: true,
            },
            "password": schema.StringAttribute{
                Optional:  true,
                Sensitive: true,
            },
        },
    }
}

// boskProviderModel maps provider schema data to a Go type.
type boskProviderModel struct {
    BaseURL  types.String `tfsdk:"baseURL"`
    Username types.String `tfsdk:"username"`
    Password types.String `tfsdk:"password"`
}

// Configure prepares a HashiCups API client for data sources and resources.
func (p *boskProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
    // Retrieve provider data from configuration
    var config boskProviderModel
    diags := req.Config.Get(ctx, &config)
    resp.Diagnostics.Append(diags...)
    if resp.Diagnostics.HasError() {
        return
    }

    if config.BaseURL.IsUnknown() {
        resp.Diagnostics.AddAttributeError(
            path.Root("baseURL"),
            "Unknown base URL",
            "Unknown base URL not supported.",
        )
    }

    baseURL := config.BaseURL.ValueString();
    username := config.Username.ValueString();
    password := config.Password.ValueString();

    if resp.Diagnostics.HasError() {
        return
    }

    client := client.NewClient(baseURL, username, password)

    resp.DataSourceData = client
    resp.ResourceData = client
}


func (p *boskProvider) DataSources(_ context.Context) []func() datasource.DataSource {
  return []func() datasource.DataSource { }
}

func (p *boskProvider) Resources(_ context.Context) []func() resource.Resource {
  return []func() resource.Resource { }
}
