package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

var (
	_ resource.ResourceWithIdentity    = (*userResource)(nil)
	_ resource.ResourceWithImportState = (*userResource)(nil)
	_ resource.ResourceWithConfigure   = (*userResource)(nil)
)

type userResource struct{ client *nuclock.Client }

func newUserResource() resource.Resource { return &userResource{} }

type userModel struct {
	ID                      types.String `tfsdk:"id"`
	Email                   types.String `tfsdk:"email"`
	Username                types.String `tfsdk:"username"`
	PublicName              types.String `tfsdk:"public_name"`
	Roles                   types.Set    `tfsdk:"roles"`
	AreaIDs                 types.Set    `tfsdk:"area_ids"`
	HasSecondFactor         types.Bool   `tfsdk:"has_2fa"`
	MustChangePassword      types.Bool   `tfsdk:"must_change_password"`
	PasswordVersion         types.Int64  `tfsdk:"password_version"`
	PasswordSetAt           types.String `tfsdk:"password_set_at"`
	SecondFactorConfirmedAt types.String `tfsdk:"second_factor_confirmed_at"`
	InvitationSentAt        types.String `tfsdk:"invitation_sent_at"`
}

type userIdentity struct {
	Email types.String `tfsdk:"email"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	kept := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A member of the organisation — staff, never a client: clients are made by the " +
			"product when they are given a project.\n\n" +
			"Creating one **mails the person a password** they must change at the first sign-in. " +
			"Nothing here ever holds a password: what a plan can compare instead is " +
			"`password_version`, `password_set_at` and the second factor's state.\n\n" +
			"Destroying one **blocks** the account rather than deleting it — work outlives " +
			"employment. Declaring a blocked person again lets them back in.\n\n" +
			"The key needs the `people.write` ability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: kept},
			"email": schema.StringAttribute{
				MarkdownDescription: "Their address, and the identity: `import { identity = { email = \"…\" } }`. " +
					"Changing it replaces the person.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "A second name to sign in with. The email when left out.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       kept,
			},
			"public_name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				// Left out, it is not managed: a name somebody chose stays theirs.
				PlanModifiers: kept,
			},
			"roles": schema.SetAttribute{
				MarkdownDescription: "`admin`, `pm` (manager) and/or `executor`.",
				ElementType:         types.StringType,
				Required:            true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf("admin", "pm", "executor")),
				},
			},
			"area_ids": schema.SetAttribute{
				MarkdownDescription: "The crafts they do, as the default for every seat they are given.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"has_2fa": schema.BoolAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"must_change_password": schema.BoolAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"password_version": schema.Int64Attribute{
				MarkdownDescription: "Moves with every reissue (`nuclock_reissue_password`) and nothing else.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"password_set_at":            schema.StringAttribute{Computed: true, PlanModifiers: kept},
			"second_factor_confirmed_at": schema.StringAttribute{Computed: true, PlanModifiers: kept},
			"invitation_sent_at":         schema.StringAttribute{Computed: true, PlanModifiers: kept},
		},
	}
}

func (r *userResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"email": identityschema.StringAttribute{RequiredForImport: true},
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *userResource) body(ctx context.Context, plan userModel) map[string]any {
	var roles []string
	plan.Roles.ElementsAs(ctx, &roles, false)
	body := map[string]any{"roles": roles}
	if !plan.PublicName.IsNull() && !plan.PublicName.IsUnknown() {
		body["public_name"] = plan.PublicName.ValueString()
	}
	if !plan.Username.IsNull() && !plan.Username.IsUnknown() {
		body["username"] = plan.Username.ValueString()
	}
	if !plan.AreaIDs.IsNull() && !plan.AreaIDs.IsUnknown() {
		var ids []string
		plan.AreaIDs.ElementsAs(ctx, &ids, false)
		body["area_ids"] = orEmpty(ids)
	}
	return body
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	email := plan.Email.ValueString()
	existing, err := r.client.UserByEmail(ctx, email)
	if err != nil {
		apiError(&resp.Diagnostics, "look the person up", err)
		return
	}
	body := r.body(ctx, plan)
	var user *nuclock.User
	switch {
	case existing != nil && existing.IsActive:
		resp.Diagnostics.AddAttributeError(path.Root("email"), "Person already exists",
			email+" already has an active account. Import it instead: "+
				"import { to = nuclock_user.<name>  identity = { email = \""+email+"\" } }")
		return
	case existing != nil:
		// Blocked by an earlier destroy: let them back in. Their history,
		// their seats and their hours never went anywhere.
		body["is_active"] = true
		body["roles"] = keepClient(body["roles"].([]string), existing)
		user, err = r.client.UpdateUser(ctx, existing.ID, body)
	default:
		body["email"] = email
		user, err = r.client.CreateUser(ctx, body)
	}
	if err != nil {
		apiError(&resp.Diagnostics, "create the person", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userState(user))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, userIdentity{Email: types.StringValue(user.Email)})...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.GetUser(ctx, state.ID.ValueString())
	// Blocked is what a destroy leaves behind, so it reads as gone.
	if nuclock.IsNotFound(err) || (err == nil && !user.IsActive) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read the person", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userState(user))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, userIdentity{Email: types.StringValue(user.Email)})...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.body(ctx, plan)
	// Roles are REPLACED by the API, and a client's role is the product's:
	// asked of the server now, so a staff edit never takes a project away.
	current, err := r.client.GetUser(ctx, state.ID.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "read the person", err)
		return
	}
	body["roles"] = keepClient(body["roles"].([]string), current)
	user, err := r.client.UpdateUser(ctx, state.ID.ValueString(), body)
	if err != nil {
		apiError(&resp.Diagnostics, "update the person", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, userState(user))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, userIdentity{Email: types.StringValue(user.Email)})...)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.UpdateUser(ctx, state.ID.ValueString(), map[string]any{"is_active": false})
	if err != nil && !nuclock.IsNotFound(err) {
		apiError(&resp.Diagnostics, "block the person", err)
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" {
		var identity userIdentity
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		user, err := r.client.UserByEmail(ctx, identity.Email.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "look the person up", err)
			return
		}
		if user == nil {
			resp.Diagnostics.AddError("No such person", "No account has the address "+identity.Email.ValueString()+".")
			return
		}
		id = user.ID
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// keepClient adds the client role to roles when the person already holds it.
//
// The product gives that role to whoever owns a project, and this resource
// manages staff roles only — it never shows `client`, so sending the
// configured roles alone would quietly take every project away from a member
// of staff who is also somebody's client.
func keepClient(roles []string, current *nuclock.User) []string {
	if current == nil {
		return roles
	}
	for _, role := range current.Roles {
		if role == "client" {
			return append(roles, "client")
		}
	}
	return roles
}

func userState(u *nuclock.User) userModel {
	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		// A client's role is the product's to give; it is not a staff role here.
		if role != "client" {
			roles = append(roles, role)
		}
	}
	return userModel{
		ID:                      types.StringValue(u.ID),
		Email:                   types.StringValue(u.Email),
		Username:                optionalString(u.Username),
		PublicName:              types.StringValue(u.PublicName),
		Roles:                   stringSet(roles),
		AreaIDs:                 stringSet(u.AreaIDs),
		HasSecondFactor:         types.BoolValue(u.HasSecondFactor),
		MustChangePassword:      types.BoolValue(u.MustChangePassword),
		PasswordVersion:         types.Int64Value(u.PasswordVersion),
		PasswordSetAt:           optionalString(u.PasswordSetAt),
		SecondFactorConfirmedAt: optionalString(u.SecondFactorConfirmedAt),
		InvitationSentAt:        optionalString(u.InvitationSentAt),
	}
}
