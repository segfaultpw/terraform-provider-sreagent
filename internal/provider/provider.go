// Package provider is the sreagent Terraform provider.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/segfaultpw/terraform-provider-sreagent/internal/client"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/engine"
	"github.com/segfaultpw/terraform-provider-sreagent/internal/specs"
)

var _ provider.Provider = (*sreagentProvider)(nil)

type sreagentProvider struct{ version string }

type providerModel struct {
	BaseURL      types.String `tfsdk:"base_url"`
	APIKey       types.String `tfsdk:"api_key"`
	Organization types.String `tfsdk:"organization"`
	MaxRetries   types.Int64  `tfsdk:"max_retries"`
}

// New returns the provider factory providerserver serves.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &sreagentProvider{version: version} }
}

func (p *sreagentProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "sreagent"
	resp.Version = p.version
}

func (p *sreagentProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages SRE Agent configuration through its configuration API (/api/v1/config).",
		Attributes: map[string]schema.Attribute{
			"base_url":     schema.StringAttribute{Optional: true, Description: "Platform URL. Defaults to SREAGENT_BASE_URL, then https://sreagent.app. Must be https except for localhost."},
			"api_key":      schema.StringAttribute{Optional: true, Sensitive: true, Description: "An sre_ak_ API key holding only the api:admin scope. Defaults to SREAGENT_API_KEY."},
			"organization": schema.StringAttribute{Optional: true, Description: "The organization slug this configuration belongs to. When set, every answer must name it, so a key for another organization can never apply this plan. Defaults to SREAGENT_ORGANIZATION."},
			"max_retries":  schema.Int64Attribute{Optional: true, Description: "Retries for rate limiting and gateway errors, 0 to 10. Defaults to 4."},
		},
	}
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (p *sreagentProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, v := range map[string]interface{ IsUnknown() bool }{"base_url": m.BaseURL, "api_key": m.APIKey, "organization": m.Organization, "max_retries": m.MaxRetries} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown provider configuration", name+" must be known when the plan is made.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	retries := 4
	if !m.MaxRetries.IsNull() {
		retries = int(m.MaxRetries.ValueInt64())
	}
	org := firstSet(m.Organization.ValueString(), os.Getenv("SREAGENT_ORGANIZATION"))
	c, err := client.New(client.Config{
		BaseURL:      firstSet(m.BaseURL.ValueString(), os.Getenv("SREAGENT_BASE_URL")),
		APIKey:       firstSet(m.APIKey.ValueString(), os.Getenv("SREAGENT_API_KEY")),
		Organization: org,
		UserAgent:    fmt.Sprintf("terraform-provider-sreagent/%s (terraform/%s; +https://registry.terraform.io/providers/segfaultpw/sreagent)", p.version, req.TerraformVersion),
		MaxRetries:   retries,
	})
	if err != nil {
		resp.Diagnostics.AddError("Invalid sreagent provider configuration", err.Error())
		return
	}
	if org != "" {
		// One read before any write, so a mismatched key fails before it can change anything.
		if _, err := c.Do(ctx, client.Request{Method: http.MethodGet, Path: "organization_settings"}); err != nil {
			resp.Diagnostics.AddError("Could not confirm the organization", err.Error())
			return
		}
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *sreagentProvider) Resources(context.Context) []func() resource.Resource {
	return engine.Resources(specs.All())
}

func (p *sreagentProvider) DataSources(context.Context) []func() datasource.DataSource {
	return engine.DataSources(specs.All())
}
