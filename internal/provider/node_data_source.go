// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &NodeDataSource{}

func NewNodeDataSource() datasource.DataSource {
	return &NodeDataSource{}
}

// NodeDataSource defines the data source implementation.
type NodeDataSource struct {
	client *http.Client
}

// NodeDataSourceModel describes the data source data model.
type NodeDataSourceModel struct {
	URL        types.String `tfsdk:"url"`
	Value_json types.String `tfsdk:"value_json"`
}

func (d *NodeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (d *NodeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Bosk state tree node data source",

		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{ // TODO: Separate base URL versus path?
				MarkdownDescription: "The HTTP address of the node",
				Required:            true,
			},
			"value_json": schema.StringAttribute{
				MarkdownDescription: "The JSON-encoded contents of the node",
				Required:            true,
			},
		},
	}
}

func (d *NodeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *NodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NodeDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result_json := getAsString(d.client, data.URL.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Value_json = types.StringValue(result_json)

	tflog.Trace(ctx, "read bosk node", map[string]interface{}{
		"url": data.URL.ValueString(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// This is, in effect, bosk client code. Can be reused for the GET operation in the resource
// Portions taken from: https://github.com/hashicorp/terraform-provider-http/blob/main/internal/provider/data_source_http.go
func getAsString(client *http.Client, url string, diag *diag.Diagnostics) string {
	httpResp, err := client.Get(url)
	if err != nil {
		diag.AddError("Client Error", fmt.Sprintf("Unable to read node: %s", err))
		return "ERROR"
	}

	defer httpResp.Body.Close()

	bytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		diag.AddError(
			"Error reading response body",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return "ERROR"
	}
	if !utf8.Valid(bytes) {
		diag.AddWarning(
			"Response body is not recognized as UTF-8",
			"Terraform may not properly handle the response_body if the contents are binary.",
		)
	}

	normalized, err := normalizeJSON(bytes)
	if err != nil {
		diag.AddWarning(
			"Error normalizing JSON response",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return string(bytes)
	}

	return string(normalized)
}

func normalizeJSON(input []byte) ([]byte, error) {
	var parsed interface{}
	err := json.Unmarshal(input, &parsed)
	if err != nil {
		return input, err
	}
	result, err := json.Marshal(parsed)
	if err != nil {
		return input, err
	}
	fmt.Println("Returning result", result)
	return result, nil
}
