package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

var (
	_ resource.ResourceWithIdentity    = (*integrationResource)(nil)
	_ resource.ResourceWithImportState = (*integrationResource)(nil)
	_ resource.ResourceWithConfigure   = (*integrationResource)(nil)
)

type integrationResource struct{ client *nuclock.Client }

func newIntegrationResource() resource.Resource { return &integrationResource{} }

type integrationModel struct {
	ID                     types.String `tfsdk:"id"`
	ProjectID              types.String `tfsdk:"project_id"`
	Kind                   types.String `tfsdk:"kind"`
	BaseURL                types.String `tfsdk:"base_url"`
	ExternalProjectID      types.String `tfsdk:"external_project_id"`
	IsActive               types.Bool   `tfsdk:"is_active"`
	Account                types.String `tfsdk:"account"`
	APITokenWO             types.String `tfsdk:"api_token_wo"`
	APITokenWOVersion      types.Int64  `tfsdk:"api_token_wo_version"`
	WebhookSecretWO        types.String `tfsdk:"webhook_secret_wo"`
	WebhookSecretWOVersion types.Int64  `tfsdk:"webhook_secret_wo_version"`
	HasAPIToken            types.Bool   `tfsdk:"has_api_token"`
	HasWebhookSecret       types.Bool   `tfsdk:"has_webhook_secret"`
}

type integrationIdentity struct {
	ProjectID         types.String `tfsdk:"project_id"`
	Kind              types.String `tfsdk:"kind"`
	ExternalProjectID types.String `tfsdk:"external_project_id"`
}

func (r *integrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_integration"
}

func (r *integrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project's connection to a tracker or a forge: GitLab and GitHub report " +
			"merge requests against tasks, OpenProject, Linear, GitHub and Jira import tasks.\n\n" +
			"**No secret reaches the state.** The API token and the webhook secret are write-only " +
			"(Terraform 1.11+): sent when the connection is made, and again only when their " +
			"`_wo_version` changes. Bump the version to rotate one; sending the same value again " +
			"changes nothing on the server either.\n\n" +
			"The key needs the `integrations.write` ability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"kind": schema.StringAttribute{
				MarkdownDescription: "`gitlab`, `github`, `openproject`, `linear` or `jira`.",
				Required:            true,
				PlanModifiers:       replace,
				Validators: []validator.String{
					stringvalidator.OneOf("gitlab", "github", "openproject", "linear", "jira"),
				},
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "The tracker's own address: `https://gitlab.com`, `https://github.com`, " +
					"`https://linear.app`, a Jira site.",
				Required: true,
			},
			"external_project_id": schema.StringAttribute{
				MarkdownDescription: "The target over there: a repository (`group/repo`, `owner/repo`), " +
					"a Linear team or a Jira project key, an OpenProject project id.",
				Required: true,
			},
			"is_active": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
			"account": schema.StringAttribute{
				MarkdownDescription: "Jira only: whose token it is (their account email).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
			},
			"api_token_wo": schema.StringAttribute{
				MarkdownDescription: "The tracker's API token. Write-only: never stored in the state.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"api_token_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Change it to send `api_token_wo` again.",
				Optional:            true,
			},
			"webhook_secret_wo": schema.StringAttribute{
				MarkdownDescription: "The secret the tracker signs its webhook with. Write-only.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"webhook_secret_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Change it to send `webhook_secret_wo` again.",
				Optional:            true,
			},
			"has_api_token": schema.BoolAttribute{
				MarkdownDescription: "Whether a token is stored. The token itself is never read back.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"has_webhook_secret": schema.BoolAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *integrationResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"project_id":          identityschema.StringAttribute{RequiredForImport: true},
			"kind":                identityschema.StringAttribute{RequiredForImport: true},
			"external_project_id": identityschema.StringAttribute{RequiredForImport: true},
		},
	}
}

func (r *integrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *integrationResource) settings(plan integrationModel) map[string]any {
	return map[string]any{
		"base_url":            plan.BaseURL.ValueString(),
		"external_project_id": plan.ExternalProjectID.ValueString(),
		"is_active":           plan.IsActive.ValueBool(),
		"account":             plan.Account.ValueString(),
	}
}

// secrets adds the write-only values to body. They exist only in the CONFIG,
// never in the plan or the state, and each goes when `send` says so.
func secrets(config integrationModel, body map[string]any, sendToken, sendSecret bool) {
	if sendToken && !config.APITokenWO.IsNull() {
		body["api_token"] = config.APITokenWO.ValueString()
	}
	if sendSecret && !config.WebhookSecretWO.IsNull() {
		body["webhook_secret"] = config.WebhookSecretWO.ValueString()
	}
}

func (r *integrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.settings(plan)
	body["project_id"] = plan.ProjectID.ValueString()
	body["kind"] = plan.Kind.ValueString()
	secrets(config, body, true, true)
	integration, err := r.client.CreateIntegration(ctx, body)
	if nuclock.HasKey(err, "integration_exists") {
		resp.Diagnostics.AddError("Connection already exists",
			"This project is already connected to that target. Import it instead, by its identity: "+
				"project_id, kind and external_project_id.")
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "connect the project", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationState(integration, plan))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, integrationIdentityOf(integration))...)
}

func (r *integrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	integration, err := r.client.GetIntegration(ctx, state.ID.ValueString())
	if nuclock.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read the connection", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationState(integration, state))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, integrationIdentityOf(integration))...)
}

func (r *integrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config integrationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.settings(plan)
	// The version is the trigger: a secret goes again only when it moved.
	secrets(config, body,
		!plan.APITokenWOVersion.Equal(state.APITokenWOVersion),
		!plan.WebhookSecretWOVersion.Equal(state.WebhookSecretWOVersion))
	integration, err := r.client.UpdateIntegration(ctx, state.ID.ValueString(), body)
	if err != nil {
		apiError(&resp.Diagnostics, "update the connection", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, integrationState(integration, plan))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, integrationIdentityOf(integration))...)
}

func (r *integrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state integrationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteIntegration(ctx, state.ID.ValueString()); err != nil && !nuclock.IsNotFound(err) {
		apiError(&resp.Diagnostics, "disconnect the project", err)
	}
}

func (r *integrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" {
		var identity integrationIdentity
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		integration, err := r.client.IntegrationOf(ctx,
			identity.ProjectID.ValueString(), identity.Kind.ValueString(), identity.ExternalProjectID.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "look the connection up", err)
			return
		}
		if integration == nil {
			resp.Diagnostics.AddError("No such connection", "That project has no connection of that kind to that target.")
			return
		}
		id = integration.ID
	}
	if req.ID != "" && !importedExists(ctx, &resp.Diagnostics, "connection", id, func(ctx context.Context, id string) error {
		_, err := r.client.GetIntegration(ctx, id)
		return err
	}) {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// integrationState keeps the versions from the configuration side: they are
// the user's triggers, and the API has no idea of them.
func integrationState(i *nuclock.Integration, prior integrationModel) integrationModel {
	return integrationModel{
		ID:                     types.StringValue(i.ID),
		ProjectID:              types.StringValue(i.ProjectID),
		Kind:                   types.StringValue(i.Kind),
		BaseURL:                types.StringValue(i.BaseURL),
		ExternalProjectID:      types.StringValue(i.ExternalProjectID),
		IsActive:               types.BoolValue(i.IsActive),
		Account:                types.StringValue(i.Account),
		APITokenWO:             types.StringNull(),
		APITokenWOVersion:      prior.APITokenWOVersion,
		WebhookSecretWO:        types.StringNull(),
		WebhookSecretWOVersion: prior.WebhookSecretWOVersion,
		HasAPIToken:            types.BoolValue(i.HasAPIToken),
		HasWebhookSecret:       types.BoolValue(i.HasWebhookSecret),
	}
}

func integrationIdentityOf(i *nuclock.Integration) integrationIdentity {
	return integrationIdentity{
		ProjectID:         types.StringValue(i.ProjectID),
		Kind:              types.StringValue(i.Kind),
		ExternalProjectID: types.StringValue(i.ExternalProjectID),
	}
}
