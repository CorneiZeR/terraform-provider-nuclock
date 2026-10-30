package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

var (
	_ resource.ResourceWithIdentity    = (*seatResource)(nil)
	_ resource.ResourceWithImportState = (*seatResource)(nil)
	_ resource.ResourceWithConfigure   = (*seatResource)(nil)
)

type seatResource struct{ client *nuclock.Client }

func newSeatResource() resource.Resource { return &seatResource{} }

type seatModel struct {
	ID         types.String `tfsdk:"id"`
	ProjectID  types.String `tfsdk:"project_id"`
	UserID     types.String `tfsdk:"user_id"`
	Position   types.String `tfsdk:"position"`
	WorkFormat types.String `tfsdk:"work_format"`
	AreaIDs    types.Set    `tfsdk:"area_ids"`
	CuratorID  types.String `tfsdk:"curator_id"`
}

type seatIdentity struct {
	ProjectID types.String `tfsdk:"project_id"`
	UserID    types.String `tfsdk:"user_id"`
}

func (r *seatResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_seat"
}

func (r *seatResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A person's seat on a project: what puts somebody on it.\n\n" +
			"**Rates are not here, on purpose.** What a seat pays and sells for stays in the " +
			"tracker; this resource never sends one and never reads one into the state. A new seat " +
			"takes the person's own base rate and the project's default sale price — a project " +
			"with no default sale price refuses the seat until somebody sets one.\n\n" +
			"Destroying one **closes** the seat: the person stops seeing the project and can take " +
			"no new work there, and every task, hour and invoice behind the seat stays. Declaring " +
			"a closed seat again reopens it.\n\n" +
			"The key needs the `seats.write` ability.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"user_id":    schema.StringAttribute{Required: true, PlanModifiers: replace},
			"position": schema.StringAttribute{
				MarkdownDescription: "Their job here, e.g. `Backend developer`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"work_format": schema.StringAttribute{
				MarkdownDescription: "`hour`, `part` (part-time) or `full` (full-time).",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:          []validator.String{stringvalidator.OneOf("hour", "part", "full")},
			},
			"area_ids": schema.SetAttribute{
				MarkdownDescription: "The crafts this seat may label work with. The person's own defaults when left out.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"curator_id": schema.StringAttribute{
				MarkdownDescription: "Who oversees them here.",
				Optional:            true,
			},
		},
	}
}

func (r *seatResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"project_id": identityschema.StringAttribute{RequiredForImport: true},
			"user_id":    identityschema.StringAttribute{RequiredForImport: true},
		},
	}
}

func (r *seatResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// terms is everything a seat says about the work, and nothing about money.
func (r *seatResource) terms(ctx context.Context, plan seatModel) map[string]any {
	// Left out, position and work format are not managed: an import keeps them.
	body := map[string]any{"curator_id": nil}
	if !plan.Position.IsNull() && !plan.Position.IsUnknown() {
		body["position"] = plan.Position.ValueString()
	}
	if !plan.WorkFormat.IsNull() && !plan.WorkFormat.IsUnknown() {
		body["work_format"] = plan.WorkFormat.ValueString()
	}
	if !plan.CuratorID.IsNull() {
		body["curator_id"] = plan.CuratorID.ValueString()
	}
	if !plan.AreaIDs.IsNull() && !plan.AreaIDs.IsUnknown() {
		var ids []string
		plan.AreaIDs.ElementsAs(ctx, &ids, false)
		body["area_ids"] = orEmpty(ids)
	}
	return body
}

func (r *seatResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan seatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, userID := plan.ProjectID.ValueString(), plan.UserID.ValueString()
	existing, err := r.client.SeatOf(ctx, projectID, userID)
	if err != nil {
		apiError(&resp.Diagnostics, "look the seat up", err)
		return
	}
	var seat *nuclock.Seat
	switch {
	case existing != nil && existing.ClosedAt == nil:
		resp.Diagnostics.AddError("Seat already exists",
			"This person already sits on this project. Import the seat instead: "+
				"import { to = nuclock_seat.<name>  identity = { project_id = \""+projectID+"\", user_id = \""+userID+"\" } }")
		return
	case existing != nil:
		// Closed by an earlier destroy: the same seat, open again, with its history.
		if _, err = r.client.ReopenSeat(ctx, existing.ID); err == nil {
			seat, err = r.client.UpdateSeat(ctx, existing.ID, r.terms(ctx, plan))
		}
	default:
		body := r.terms(ctx, plan)
		body["project_id"] = projectID
		body["user_id"] = userID
		seat, err = r.client.CreateSeat(ctx, body)
	}
	if err != nil {
		apiError(&resp.Diagnostics, "seat the person", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, seatState(seat))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, seatIdentity{ProjectID: types.StringValue(seat.ProjectID), UserID: types.StringValue(seat.UserID)})...)
}

func (r *seatResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state seatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	seat, err := r.client.GetSeat(ctx, state.ID.ValueString())
	// Closed is what a destroy leaves behind, so it reads as gone.
	if nuclock.IsNotFound(err) || (err == nil && seat.ClosedAt != nil) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read the seat", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, seatState(seat))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, seatIdentity{ProjectID: types.StringValue(seat.ProjectID), UserID: types.StringValue(seat.UserID)})...)
}

func (r *seatResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state seatModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	seat, err := r.client.UpdateSeat(ctx, state.ID.ValueString(), r.terms(ctx, plan))
	if err != nil {
		apiError(&resp.Diagnostics, "update the seat", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, seatState(seat))...)
	resp.Diagnostics.Append(resp.Identity.Set(ctx, seatIdentity{ProjectID: types.StringValue(seat.ProjectID), UserID: types.StringValue(seat.UserID)})...)
}

func (r *seatResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state seatModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client.CloseSeat(ctx, state.ID.ValueString()); err != nil && !nuclock.IsNotFound(err) {
		apiError(&resp.Diagnostics, "close the seat", err)
	}
}

func (r *seatResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" {
		var identity seatIdentity
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		seat, err := r.client.SeatOf(ctx, identity.ProjectID.ValueString(), identity.UserID.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "look the seat up", err)
			return
		}
		if seat == nil {
			resp.Diagnostics.AddError("No such seat", "That person holds no seat on that project.")
			return
		}
		id = seat.ID
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func seatState(s *nuclock.Seat) seatModel {
	return seatModel{
		ID:         types.StringValue(s.ID),
		ProjectID:  types.StringValue(s.ProjectID),
		UserID:     types.StringValue(s.UserID),
		Position:   types.StringValue(s.Position),
		WorkFormat: types.StringValue(s.WorkFormat),
		AreaIDs:    stringSet(s.AreaIDs),
		CuratorID:  optionalString(s.CuratorID),
	}
}
