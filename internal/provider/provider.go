// Copyright (c) 2026 Patrick Doyle
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/prdoyle/terraform-provider-bosk/internal/bosk"
)

var _ provider.Provider = &BoskProvider{}

// BoskProvider defines the provider implementation.
type BoskProvider struct {
	version string
}

// BoskProviderModel describes the provider configuration.
type BoskProviderModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	Token              types.String `tfsdk:"token"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	Headers            types.Map    `tfsdk:"headers"`
	RequestTimeout     types.String `tfsdk:"request_timeout"`
	CACert             types.String `tfsdk:"ca_cert"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
}

func (p *BoskProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "bosk"
	resp.Version = p.version
}

func (p *BoskProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Provides access to a bosk maintenance endpoint. This API exposes full read and write access to the bosk state tree, so use credentials with the smallest appropriate privileges.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the bosk maintenance endpoints, for example `https://app.example.com/bosk/state`. May also be set with the `BOSK_ENDPOINT` environment variable.",
				Optional:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Bearer token sent in the `Authorization` header. May also be set with the `BOSK_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for HTTP Basic authentication. May also be set with the `BOSK_USERNAME` environment variable.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for HTTP Basic authentication. May also be set with the `BOSK_PASSWORD` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"headers": schema.MapAttribute{
				MarkdownDescription: "Additional HTTP headers to send with every request.",
				Optional:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
			},
			"request_timeout": schema.StringAttribute{
				MarkdownDescription: "Timeout for each HTTP request, as a duration string such as `10s`. Defaults to `10s`. May also be set with the `BOSK_REQUEST_TIMEOUT` environment variable.",
				Optional:            true,
			},
			"ca_cert": schema.StringAttribute{
				MarkdownDescription: "PEM-encoded CA certificate to trust in addition to the system roots. May also be set with the `BOSK_CA_CERT` environment variable.",
				Optional:            true,
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. This is insecure and should only be used for local development. May also be set with the `BOSK_INSECURE_SKIP_VERIFY` environment variable.",
				Optional:            true,
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

	endpointValue, ok := stringValueOrEnv(data.Endpoint, "BOSK_ENDPOINT", path.Root("endpoint"), &resp.Diagnostics)
	if !ok {
		return
	}
	if endpointValue == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing bosk endpoint",
			"Set the provider's endpoint attribute or the BOSK_ENDPOINT environment variable.",
		)
		return
	}

	endpoint, err := url.Parse(endpointValue)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Invalid bosk endpoint", err.Error())
		return
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Invalid bosk endpoint", "The endpoint must use http or https.")
		return
	}
	if endpoint.Host == "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Invalid bosk endpoint", "The endpoint must include a host.")
		return
	}
	if endpoint.RawQuery != "" || endpoint.Fragment != "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Invalid bosk endpoint", "The endpoint must not include a query or fragment.")
		return
	}

	token, ok := stringValueOrEnv(data.Token, "BOSK_TOKEN", path.Root("token"), &resp.Diagnostics)
	if !ok {
		return
	}
	username, ok := stringValueOrEnv(data.Username, "BOSK_USERNAME", path.Root("username"), &resp.Diagnostics)
	if !ok {
		return
	}
	password, ok := stringValueOrEnv(data.Password, "BOSK_PASSWORD", path.Root("password"), &resp.Diagnostics)
	if !ok {
		return
	}
	if token != "" && (username != "" || password != "") {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Conflicting authentication settings",
			"Set either token or username/password, not both.",
		)
		return
	}
	if (username == "") != (password == "") {
		resp.Diagnostics.AddAttributeError(
			path.Root("username"),
			"Incomplete basic authentication settings",
			"Set both username and password, or neither.",
		)
		return
	}

	headers := map[string]string{}
	if !data.Headers.IsNull() && !data.Headers.IsUnknown() {
		for name, value := range data.Headers.Elements() {
			stringValue, ok := value.(types.String)
			if !ok {
				resp.Diagnostics.AddAttributeError(path.Root("headers"), "Invalid header value", fmt.Sprintf("Header %q must have a string value.", name))
				return
			}
			headers[name] = stringValue.ValueString()
		}
	}

	timeoutValue, ok := stringValueOrEnv(data.RequestTimeout, "BOSK_REQUEST_TIMEOUT", path.Root("request_timeout"), &resp.Diagnostics)
	if !ok {
		return
	}
	if timeoutValue == "" {
		timeoutValue = "10s"
	}
	requestTimeout, err := time.ParseDuration(timeoutValue)
	if err != nil || requestTimeout <= 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("request_timeout"),
			"Invalid request timeout",
			fmt.Sprintf("Use a positive duration such as \"10s\"; got %q.", timeoutValue),
		)
		return
	}

	caCert, ok := stringValueOrEnv(data.CACert, "BOSK_CA_CERT", path.Root("ca_cert"), &resp.Diagnostics)
	if !ok {
		return
	}

	insecureSkipVerify := false
	if !data.InsecureSkipVerify.IsNull() && !data.InsecureSkipVerify.IsUnknown() {
		insecureSkipVerify = data.InsecureSkipVerify.ValueBool()
	} else if value, exists := os.LookupEnv("BOSK_INSECURE_SKIP_VERIFY"); exists && value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("insecure_skip_verify"),
				"Invalid insecure_skip_verify value",
				fmt.Sprintf("BOSK_INSECURE_SKIP_VERIFY must be true or false; got %q.", value),
			)
			return
		}
		insecureSkipVerify = parsed
	}

	if endpoint.Scheme == "http" && (token != "" || username != "") {
		resp.Diagnostics.AddWarning(
			"Credentials sent over HTTP",
			"The bosk endpoint uses http, so credentials will be sent in cleartext. Use https for anything other than local development.",
		)
	}

	userAgent := "terraform-provider-bosk/" + p.version
	if req.TerraformVersion != "" {
		userAgent += " terraform/" + req.TerraformVersion
	}

	client, err := bosk.NewClient(bosk.Config{
		Endpoint:           endpoint,
		Token:              token,
		Username:           username,
		Password:           password,
		Headers:            headers,
		RequestTimeout:     requestTimeout,
		CACert:             caCert,
		InsecureSkipVerify: insecureSkipVerify,
		UserAgent:          userAgent,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure bosk client", err.Error())
		return
	}

	store := bosk.NewNodeStore(client)
	resp.ResourceData = store
	resp.DataSourceData = store

	tflog.Debug(ctx, "Configured bosk provider", map[string]any{
		"endpoint": endpoint.String(),
	})
}

func (p *BoskProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNodeResource,
	}
}

func (p *BoskProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &BoskProvider{version: version}
	}
}

func stringValueOrEnv(value types.String, envName string, attrPath path.Path, diagnostics *diag.Diagnostics) (string, bool) {
	if value.IsUnknown() {
		diagnostics.AddAttributeError(attrPath, "Unknown configuration value", "This value must be known before the provider can be configured.")
		return "", false
	}
	if !value.IsNull() && value.ValueString() != "" {
		return value.ValueString(), true
	}
	return os.Getenv(envName), true
}
