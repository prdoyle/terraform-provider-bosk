// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
	client *BoskClient
}

func (d *NodeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (d *NodeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Bosk state tree node data source",

		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "Specifies the HTTP address of URL of the bosk node.",
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

	client, ok := req.ProviderData.(*BoskClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *BoskClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *NodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NodeModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result_json := d.client.GetJSONAsString(data.URL.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data.Value_json = types.StringValue(result_json)

	tflog.Debug(ctx, "read bosk node datasource", map[string]interface{}{
		"url": data.URL.ValueString(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
