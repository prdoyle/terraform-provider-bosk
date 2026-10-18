// Copyright (c) 2026 Patrick Doyle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
	"github.com/prdoyle/terraform-provider-bosk/internal/bosk/jsonutil"
	"github.com/prdoyle/terraform-provider-bosk/internal/bosk/pathutil"
)

const defaultOperationTimeout = time.Minute

var (
	_ resource.Resource                = &NodeResource{}
	_ resource.ResourceWithImportState = &NodeResource{}
	_ resource.ResourceWithIdentity    = &NodeResource{}
)

type NodeResource struct {
	store *bosk.NodeStore
}

type NodeResourceModel struct {
	Path     types.String         `tfsdk:"path"`
	Value    jsontypes.Normalized `tfsdk:"value"`
	ETag     types.String         `tfsdk:"etag"`
	Timeouts timeouts.Value       `tfsdk:"timeouts"`
}

func NewNodeResource() resource.Resource {
	return &NodeResource{}
}

func (r *NodeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node"
}

func (r *NodeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the value at a path in a bosk state tree. A node is a path/value slot; create fails if the path already exists, update replaces the value, and delete removes it. For nodes with a `revision` field, include it in `value`; the provider uses the corresponding ETag for conditional writes.",

		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				MarkdownDescription: "Path of the node below the provider's maintenance endpoint, beginning with `/`. Path segments must be percent-encoded. Changing this forces a new resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					resourcePathValidator{},
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "JSON-encoded value of the node. Use `jsonencode(...)` to build it.",
				Required:            true,
				CustomType:          jsontypes.NormalizedType{},
				PlanModifiers: []planmodifier.String{
					jsonSemanticEqualityPlanModifier{},
				},
			},
			"etag": schema.StringAttribute{
				MarkdownDescription: "Opaque concurrency token returned by bosk for nodes that have a `revision` field. Terraform sends it back in `If-Match` on update and delete.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					etagPlanModifier{},
				},
			},
			"timeouts": timeouts.AttributesAll(ctx),
		},
	}
}

func (r *NodeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"path": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "Path of the bosk node.",
			},
		},
	}
}

func (r *NodeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	store, ok := req.ProviderData.(*bosk.NodeStore)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected resource configure type",
			fmt.Sprintf("Expected *bosk.NodeStore, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.store = store
}

func (r *NodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := data.Timeouts.Create(ctx, defaultOperationTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	node, err := r.store.Create(ctx, data.Path.ValueString(), []byte(data.Value.ValueString()))
	if err != nil {
		addNodeError(&resp.Diagnostics, "create", data.Path.ValueString(), err)
		return
	}

	setNodeModel(&data, node)
	setNodeIdentity(ctx, resp.Identity, node.Path, &resp.Diagnostics)
	tflog.Debug(ctx, "Created bosk node", map[string]any{"path": node.Path})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := data.Timeouts.Read(ctx, defaultOperationTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	node, err := r.store.Read(ctx, data.Path.ValueString())
	if err != nil {
		if bosk.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addNodeError(&resp.Diagnostics, "read", data.Path.ValueString(), err)
		return
	}

	setNodeModel(&data, node)
	setNodeIdentity(ctx, resp.Identity, node.Path, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var prior NodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := plan.Timeouts.Update(ctx, defaultOperationTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	etag := ""
	if !prior.ETag.IsNull() && !prior.ETag.IsUnknown() {
		etag = prior.ETag.ValueString()
	}

	node, err := r.store.Update(ctx, plan.Path.ValueString(), []byte(plan.Value.ValueString()), etag)
	if err != nil {
		addNodeError(&resp.Diagnostics, "update", plan.Path.ValueString(), err)
		return
	}

	setNodeModel(&plan, node)
	setNodeIdentity(ctx, resp.Identity, node.Path, &resp.Diagnostics)
	tflog.Debug(ctx, "Updated bosk node", map[string]any{"path": node.Path})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := data.Timeouts.Delete(ctx, defaultOperationTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	etag := ""
	if !data.ETag.IsNull() && !data.ETag.IsUnknown() {
		etag = data.ETag.ValueString()
	}

	if err := r.store.Delete(ctx, data.Path.ValueString(), etag); err != nil {
		addNodeError(&resp.Diagnostics, "delete", data.Path.ValueString(), err)
		return
	}

	tflog.Debug(ctx, "Deleted bosk node", map[string]any{"path": data.Path.ValueString()})
}

func (r *NodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughWithIdentity(ctx, path.Root("path"), path.Root("path"), req, resp)
}

func setNodeIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, nodePath string, diagnostics *diag.Diagnostics) {
	if identity == nil {
		return
	}
	diagnostics.Append(identity.SetAttribute(ctx, path.Root("path"), types.StringValue(nodePath))...)
}

func setNodeModel(model *NodeResourceModel, node *bosk.Node) {
	model.Path = types.StringValue(node.Path)
	model.Value = jsontypes.NewNormalizedValue(string(node.Value))
	if node.ETag == "" {
		model.ETag = types.StringNull()
	} else {
		model.ETag = types.StringValue(node.ETag)
	}
}

func addNodeError(diagnostics *diag.Diagnostics, action, nodePath string, err error) {
	var alreadyExists *bosk.AlreadyExistsError
	var conflict *bosk.ConflictError
	var createFailed *bosk.CreateFailedError
	var apiErr *bosk.APIError
	var notFound *bosk.NotFoundError

	switch {
	case errors.As(err, &alreadyExists):
		diagnostics.AddError(
			"bosk node already exists",
			fmt.Sprintf("A node already exists at %s. To manage it with Terraform, import it first:\n\n  terraform import <resource-address> %s", nodePath, nodePath),
		)
	case errors.As(err, &conflict):
		detail := fmt.Sprintf("The node at %s changed concurrently. %s\n\nRun `terraform plan` again to reconcile the latest value.", nodePath, conflict.Reason)
		if conflict.Observed != nil {
			detail += fmt.Sprintf("\n\nObserved value: %s", truncateForDiagnostic(conflict.Observed.Value))
		}
		diagnostics.AddError("bosk node changed concurrently", detail)
	case errors.As(err, &createFailed):
		diagnostics.AddError(
			"bosk node was not created",
			fmt.Sprintf("bosk accepted the create for %s, but the node is not present. Check that the parent path exists and that the value matches the state tree's type.", nodePath),
		)
	case errors.As(err, &notFound):
		diagnostics.AddError("bosk node not found", fmt.Sprintf("No node exists at %s.", nodePath))
	case errors.As(err, &apiErr):
		detail := fmt.Sprintf("%s %s returned %d %s.", apiErr.Method, apiErr.URL, apiErr.StatusCode, apiErr.Status)
		if apiErr.Body != "" {
			detail += "\n\n" + apiErr.Body
		}
		diagnostics.AddError("bosk API error", detail)
	default:
		diagnostics.AddError("bosk "+action+" failed", err.Error())
	}
}

func truncateForDiagnostic(value []byte) string {
	const maxBytes = 2048
	if len(value) <= maxBytes {
		return string(value)
	}
	return string(value[:maxBytes]) + "..."
}

// jsonSemanticEqualityPlanModifier preserves the prior value when the
// configuration is semantically equal JSON. The framework applies custom-type
// semantic equality after read/create/update, but not during planning, so
// without this a whitespace or key-order change would show a spurious diff.
type jsonSemanticEqualityPlanModifier struct{}

func (jsonSemanticEqualityPlanModifier) Description(context.Context) string {
	return "treats semantically equal JSON values as unchanged"
}

func (jsonSemanticEqualityPlanModifier) MarkdownDescription(context.Context) string {
	return "treats semantically equal JSON values as unchanged"
}

func (jsonSemanticEqualityPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	equal, err := jsonutil.Equal([]byte(req.StateValue.ValueString()), []byte(req.PlanValue.ValueString()))
	if err != nil {
		// Validation will report malformed JSON separately.
		return
	}
	if equal {
		resp.PlanValue = req.StateValue
	}
}

// etagPlanModifier preserves the prior ETag when the value is semantically
// unchanged. Without it Terraform would show a spurious "(known after apply)"
// on plans that change nothing.
type etagPlanModifier struct{}

func (etagPlanModifier) Description(context.Context) string {
	return "preserves the prior ETag when the node value is unchanged"
}

func (etagPlanModifier) MarkdownDescription(context.Context) string {
	return "preserves the prior ETag when the node value is unchanged"
}

func (etagPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsUnknown() {
		return
	}

	var stateValue jsontypes.Normalized
	if diags := req.State.GetAttribute(ctx, path.Root("value"), &stateValue); diags.HasError() {
		return
	}
	var planValue jsontypes.Normalized
	if diags := req.Plan.GetAttribute(ctx, path.Root("value"), &planValue); diags.HasError() {
		return
	}
	if stateValue.IsNull() || stateValue.IsUnknown() || planValue.IsNull() || planValue.IsUnknown() {
		return
	}

	equal, err := jsonutil.Equal([]byte(stateValue.ValueString()), []byte(planValue.ValueString()))
	if err != nil || !equal {
		return
	}

	resp.PlanValue = req.StateValue
}

type resourcePathValidator struct{}

func (resourcePathValidator) Description(context.Context) string {
	return "must be a non-root bosk path beginning with `/`"
}

func (resourcePathValidator) MarkdownDescription(context.Context) string {
	return "must be a non-root bosk path beginning with `/`"
}

func (resourcePathValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := pathutil.ValidateResourcePath(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid bosk path", err.Error())
	}
}
