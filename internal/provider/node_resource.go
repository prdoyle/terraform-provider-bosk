// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NodeResource{}
var _ resource.ResourceWithImportState = &NodeResource{}

func NewNodeResource() resource.Resource {
	return &NodeResource{}
}

// NodeResource defines the resource implementation.
type NodeResource struct {
	client *BoskClient
}

func (r *NodeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (r *NodeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (r *NodeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.client = client
}

func (r NodeModel) url() string {
	return r.URL.ValueString()
}

func (r *NodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Error getting plan data", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	data.Validate(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Invalid plan", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	r.client.PutJSONAsString(data.url(), data.Value_json.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Error performing PUT", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	tflog.Debug(ctx, "created bosk node", map[string]interface{}{
		"url": data.url(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NodeModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Error getting plan data", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}
	data.Validate(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Invalid state", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	result_json := r.client.GetJSONAsString(data.url(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Error performing GET", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	data.Value_json = types.StringValue(result_json)

	tflog.Debug(ctx, "read bosk node", map[string]interface{}{
		"url": data.url(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Error setting state", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}
}

func (r *NodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Validate(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Invalid plan", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	r.client.PutJSONAsString(data.url(), data.Value_json.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "updated bosk node", map[string]interface{}{
		"url": data.url(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NodeModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Validate(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		tflog.Warn(ctx, "Invalid state", map[string]interface{}{"diagnostics": resp.Diagnostics})
		return
	}

	r.client.Delete(data.url(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "deleted bosk node", map[string]interface{}{
		"url": data.url(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	url := req.ID

	result_json := r.client.GetJSONAsString(url, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	data := NodeModel{
		URL:        types.StringValue(url),
		Value_json: types.StringValue(result_json),
	}

	tflog.Debug(ctx, "imported bosk node", map[string]interface{}{
		"url": data.url(),
	})

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
