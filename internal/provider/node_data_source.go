// Copyright (c) 2026 Patrick Doyle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
	"github.com/prdoyle/terraform-provider-bosk/internal/bosk/pathutil"
)

var _ datasource.DataSource = &NodeDataSource{}

type NodeDataSource struct {
	store *bosk.NodeStore
}

type NodeDataSourceModel struct {
	Path  types.String         `tfsdk:"path"`
	Value jsontypes.Normalized `tfsdk:"value"`
	ETag  types.String         `tfsdk:"etag"`
}

func NewNodeDataSource() datasource.DataSource {
	return &NodeDataSource{}
}

func (d *NodeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (d *NodeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the value at a path in a bosk state tree.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				MarkdownDescription: "Path of the node below the provider's maintenance endpoint, beginning with `/`.",
				Required:            true,
				Validators: []validator.String{
					dataSourcePathValidator{},
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded value of the node.",
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
			"etag": schema.StringAttribute{
				MarkdownDescription: "Opaque ETag returned by bosk for nodes that have a `revision` field.",
				Computed:            true,
			},
		},
	}
}

func (d *NodeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	store, ok := req.ProviderData.(*bosk.NodeStore)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected data source configure type",
			fmt.Sprintf("Expected *bosk.NodeStore, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.store = store
}

func (d *NodeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NodeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	node, err := d.store.Read(ctx, data.Path.ValueString())
	if err != nil {
		if bosk.IsNotFound(err) {
			resp.Diagnostics.AddAttributeError(path.Root("path"), "bosk node not found", fmt.Sprintf("No node exists at %s.", data.Path.ValueString()))
			return
		}
		addNodeError(&resp.Diagnostics, "read", data.Path.ValueString(), err)
		return
	}

	data.Path = types.StringValue(node.Path)
	data.Value = jsontypes.NewNormalizedValue(string(node.Value))
	if node.ETag == "" {
		data.ETag = types.StringNull()
	} else {
		data.ETag = types.StringValue(node.ETag)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type dataSourcePathValidator struct{}

func (dataSourcePathValidator) Description(context.Context) string {
	return "must be a bosk path beginning with `/`"
}

func (dataSourcePathValidator) MarkdownDescription(context.Context) string {
	return "must be a bosk path beginning with `/`"
}

func (dataSourcePathValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := pathutil.Validate(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid bosk path", err.Error())
	}
}
