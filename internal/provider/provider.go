// Package provider is the Terraform side of nuclock: schemas, plans and state.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

var (
	_ provider.ProviderWithActions       = (*nuclockProvider)(nil)
	_ provider.ProviderWithListResources = (*nuclockProvider)(nil)
)

type nuclockProvider struct {
	version string
}

// New returns the provider factory for the given release version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &nuclockProvider{version: version} }
}

type providerModel struct {
	URL    types.String `tfsdk:"url"`
	APIKey types.String `tfsdk:"api_key"`
}

func (p *nuclockProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "nuclock"
	resp.Version = p.version
}

func (p *nuclockProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a nuclock organisation as code: its projects, its people, " +
			"who sits on which project, and the trackers each project is connected to.\n\n" +
			"Authenticate with an organisation's **service key** (Settings → Assistants → " +
			"Organisation keys, scope `write`). A key holds only the abilities it was minted " +
			"with; each resource names the one it needs, and a key without it is refused with " +
			"`ability_missing` naming that ability.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				MarkdownDescription: "The API's address, e.g. `https://api.clock.example.com`. " +
					"May be set with `NUCLOCK_URL`.",
				Optional: true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "A service key. May be set with `NUCLOCK_API_KEY`.",
				Optional:            true,
				Sensitive:           true,
			},
		},
	}
}

func (p *nuclockProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	url := os.Getenv("NUCLOCK_URL")
	if !config.URL.IsNull() {
		url = config.URL.ValueString()
	}
	key := os.Getenv("NUCLOCK_API_KEY")
	if !config.APIKey.IsNull() {
		key = config.APIKey.ValueString()
	}
	if url == "" {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "No nuclock address",
			"Set `url` in the provider block or NUCLOCK_URL in the environment.")
	}
	if key == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "No nuclock key",
			"Set `api_key` in the provider block or NUCLOCK_API_KEY in the environment.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	client := nuclock.New(url, key, "terraform-provider-nuclock/"+p.version)
	resp.ResourceData = client
	resp.DataSourceData = client
	resp.ActionData = client
	resp.ListResourceData = client
}

func (p *nuclockProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newProjectResource,
		newUserResource,
		newSeatResource,
		newIntegrationResource,
	}
}

func (p *nuclockProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newProjectDataSource,
		newUserDataSource,
		newSeatDataSource,
	}
}

func (p *nuclockProvider) Actions(_ context.Context) []func() action.Action {
	return []func() action.Action{
		newReissuePasswordAction,
		newResetSecondFactorAction,
		newResendInvitationAction,
	}
}

func (p *nuclockProvider) ListResources(_ context.Context) []func() list.ListResource {
	return []func() list.ListResource{
		newProjectListResource,
		newUserListResource,
		newSeatListResource,
	}
}
