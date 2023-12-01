package provider

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type NodeModel struct {
	URL        types.String `tfsdk:"url"`
	Value_json types.String `tfsdk:"value_json"`
}

func (m *NodeModel) Validate(diag *diag.Diagnostics) {
	var url string = m.URL.ValueString()
	if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		diag.AddError(
			"URL must be http or https",
			fmt.Sprintf("Expected url field to start with either \"http://\" or \"https://\". Got: %v", url),
		)
	}
}
