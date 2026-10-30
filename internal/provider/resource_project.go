package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

var (
	_ resource.ResourceWithIdentity    = (*projectResource)(nil)
	_ resource.ResourceWithImportState = (*projectResource)(nil)
	_ resource.ResourceWithConfigure   = (*projectResource)(nil)
)

type projectResource struct{ client *nuclock.Client }

func newProjectResource() resource.Resource { return &projectResource{} }

type projectModel struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	KeyPrefix   types.String `tfsdk:"key_prefix"`
	Currency    types.String `tfsdk:"currency"`
	ManagerIDs  types.Set    `tfsdk:"manager_ids"`
	OwnerIDs    types.Set    `tfsdk:"owner_ids"`
}

type projectIdentity struct {
	KeyPrefix types.String `tfsdk:"key_prefix"`
}

func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project: the unit work, hours and invoices belong to.\n\n" +
			"Destroying one **archives** it — it takes no new work, and its hours, invoices and " +
			"payouts stay. Declaring an archived project again brings it back. Money — the agency " +
			"fee and the default sale price — is not managed here; it stays in the tracker.\n\n" +
			"The key needs the `projects.write` ability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"title": schema.StringAttribute{Required: true},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				// Left out, it is not managed: an import keeps what is there.
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key_prefix": schema.StringAttribute{
				MarkdownDescription: "What every task key here leads with (`NUC` in `NUC-142`). Guessed " +
					"from the title when left out. The project's identity: `import { identity = { key_prefix = \"NUC\" } }`.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"currency": schema.StringAttribute{
				MarkdownDescription: "What the project is billed in, e.g. `USD`. The organisation's default when left out.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"owner_ids": schema.SetAttribute{
				MarkdownDescription: "The clients it belongs to — their accounts are the product's, made " +
					"when they are given a project; look them up with the `nuclock_user` data source. " +
					"Left out, the owners are not managed here.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"manager_ids": schema.SetAttribute{
				MarkdownDescription: "The people who run it (users holding the manager role).",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *projectResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"key_prefix": identityschema.StringAttribute{RequiredForImport: true},
		},
	}
}

func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *projectResource) body(ctx context.Context, plan projectModel) map[string]any {
	body := map[string]any{"title": plan.Title.ValueString()}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body["description"] = plan.Description.ValueString()
	}
	if !plan.KeyPrefix.IsNull() && !plan.KeyPrefix.IsUnknown() {
		body["key_prefix"] = plan.KeyPrefix.ValueString()
	}
	if !plan.Currency.IsNull() && !plan.Currency.IsUnknown() {
		body["currency"] = plan.Currency.ValueString()
	}
	if !plan.OwnerIDs.IsNull() && !plan.OwnerIDs.IsUnknown() {
		var ids []string
		plan.OwnerIDs.ElementsAs(ctx, &ids, false)
		body["owner_ids"] = orEmpty(ids)
	}
	if !plan.ManagerIDs.IsNull() && !plan.ManagerIDs.IsUnknown() {
		var ids []string
		plan.ManagerIDs.ElementsAs(ctx, &ids, false)
		body["manager_ids"] = orEmpty(ids)
	}
	return body
}

func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.body(ctx, plan)
	var project *nuclock.Project
	var err error
	// A prefix names one project. An archived one under it is the one a
	// previous destroy put away: bring it back rather than fail on a twin.
	if prefix, ok := body["key_prefix"].(string); ok && prefix != "" {
		existing, lookupErr := r.client.ProjectByKeyPrefix(ctx, prefix)
		if lookupErr != nil {
			apiError(&resp.Diagnostics, "look the project up", lookupErr)
			return
		}
		if existing != nil {
			if existing.IsActive {
				resp.Diagnostics.AddAttributeError(path.Root("key_prefix"), "Project already exists",
					"A live project already leads its keys with "+prefix+". Import it instead: "+
						"import { to = nuclock_project.<name>  identity = { key_prefix = \""+prefix+"\" } }")
				return
			}
			body["is_active"] = true
			project, err = r.client.UpdateProject(ctx, existing.ID, body)
		}
	}
	if project == nil && err == nil {
		project, err = r.client.CreateProject(ctx, body)
	}
	if err != nil {
		apiError(&resp.Diagnostics, "create the project", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, projectState(project))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, projectIdentity{KeyPrefix: types.StringValue(project.KeyPrefix)})...)
}

func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project, err := r.client.GetProject(ctx, state.ID.ValueString())
	// An archived project is what this resource leaves behind on destroy, so
	// it reads as gone: declaring it again brings it back through Create.
	if nuclock.IsNotFound(err) || (err == nil && !project.IsActive) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read the project", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, projectState(project))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, projectIdentity{KeyPrefix: types.StringValue(project.KeyPrefix)})...)
}

func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	project, err := r.client.UpdateProject(ctx, state.ID.ValueString(), r.body(ctx, plan))
	if err != nil {
		apiError(&resp.Diagnostics, "update the project", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, projectState(project))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, projectIdentity{KeyPrefix: types.StringValue(project.KeyPrefix)})...)
}

func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.UpdateProject(ctx, state.ID.ValueString(), map[string]any{"is_active": false})
	if err != nil && !nuclock.IsNotFound(err) {
		apiError(&resp.Diagnostics, "archive the project", err)
	}
}

func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" {
		var identity projectIdentity
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		project, err := r.client.ProjectByKeyPrefix(ctx, identity.KeyPrefix.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "look the project up", err)
			return
		}
		if project == nil {
			resp.Diagnostics.AddError("No such project", "No project leads its keys with "+identity.KeyPrefix.ValueString()+".")
			return
		}
		id = project.ID
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func projectState(p *nuclock.Project) projectModel {
	return projectModel{
		ID:          types.StringValue(p.ID),
		Title:       types.StringValue(p.Title),
		Description: types.StringValue(p.Description),
		KeyPrefix:   types.StringValue(p.KeyPrefix),
		Currency:    types.StringValue(p.Currency),
		ManagerIDs:  stringSet(p.ManagerIDs),
		OwnerIDs:    stringSet(p.OwnerIDs),
	}
}
