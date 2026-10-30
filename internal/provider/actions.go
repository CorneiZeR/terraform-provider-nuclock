package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

// A recovery is an ACT, not a state: it holds nothing between applies, which
// is what a Terraform action is (1.14+). Each sends the person a letter, and
// each needs `people.recovery` on the key — the one ability never in a preset.

type recoveryAction struct {
	client      *nuclock.Client
	name        string
	description string
	clientFlag  bool
	invoke      func(ctx context.Context, c *nuclock.Client, userID string, client bool) error
	doing       string
	done        string
}

var _ action.ActionWithConfigure = (*recoveryAction)(nil)

func newReissuePasswordAction() action.Action {
	return &recoveryAction{
		name: "reissue_password",
		description: "Mint a new password for a person and **mail it to them**. Their old password " +
			"stops working at once, every session they hold ends, and the first sign-in must " +
			"replace the new one. Nothing types the password and nothing echoes it — not this " +
			"action, not the state, not a log. `password_version` on `nuclock_user` moves.\n\n" +
			"The key needs the `people.recovery` ability.",
		invoke: func(ctx context.Context, c *nuclock.Client, id string, _ bool) error {
			return c.ReissuePassword(ctx, id)
		},
		doing: "reissue the password",
		done:  "A new password was mailed to the person.",
	}
}

func newResetSecondFactorAction() action.Action {
	return &recoveryAction{
		name: "reset_mfa",
		description: "Clear a person's second factor, for somebody who lost their authenticator. " +
			"**A letter tells them** it was cleared, and they enrol again at the next sign-in.\n\n" +
			"The key needs the `people.recovery` ability.",
		invoke: func(ctx context.Context, c *nuclock.Client, id string, _ bool) error {
			return c.ResetSecondFactor(ctx, id)
		},
		doing: "reset the second factor",
		done:  "The second factor was cleared.",
	}
}

func newResendInvitationAction() action.Action {
	return &recoveryAction{
		name:        "resend_invitation",
		clientFlag: true,
		description: "Send a person their invitation **letter** again — one lost to spam, or read " +
			"after it expired. A member's by default; `client = true` for a client's.\n\n" +
			"The key needs the `people.recovery` ability.",
		invoke: func(ctx context.Context, c *nuclock.Client, id string, client bool) error {
			return c.ResendInvitation(ctx, id, client)
		},
		doing: "send the invitation again",
		done:  "The invitation went out again.",
	}
}

func (a *recoveryAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + a.name
}

func (a *recoveryAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	attributes := map[string]schema.Attribute{
		"user_id": schema.StringAttribute{
			MarkdownDescription: "The person — `nuclock_user.<name>.id`.",
			Required:            true,
		},
	}
	if a.clientFlag {
		attributes["client"] = schema.BoolAttribute{
			MarkdownDescription: "Send a client's invitation rather than a member's.",
			Optional:            true,
		}
	}
	resp.Schema = schema.Schema{MarkdownDescription: a.description, Attributes: attributes}
}

func (a *recoveryAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (a *recoveryAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var userID types.String
	var client types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, pathRoot("user_id"), &userID)...)
	if a.clientFlag {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, pathRoot("client"), &client)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if err := a.invoke(ctx, a.client, userID.ValueString(), client.ValueBool()); err != nil {
		apiError(&resp.Diagnostics, a.doing, err)
		return
	}
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: a.done})
	}
}
