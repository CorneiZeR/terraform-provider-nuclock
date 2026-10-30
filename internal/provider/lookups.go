package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/CorneiZeR/terraform-provider-nuclock/internal/nuclock"
)

func pathRoot(name string) path.Path { return path.Root(name) }

// --- data sources: one object by what a configuration knows about it ---------

type lookup struct {
	client *nuclock.Client
	name   string
	about  string
	keys   map[string]dsschema.Attribute
	fields map[string]dsschema.Attribute
	read   func(ctx context.Context, c *nuclock.Client, config map[string]string) (map[string]any, error)
}

func (d *lookup) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.name
}

func (d *lookup) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]dsschema.Attribute{}
	for name, attribute := range d.keys {
		attributes[name] = attribute
	}
	for name, attribute := range d.fields {
		attributes[name] = attribute
	}
	resp.Schema = dsschema.Schema{MarkdownDescription: d.about, Attributes: attributes}
}

func (d *lookup) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *lookup) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	config := map[string]string{}
	for name := range d.keys {
		var value types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(name), &value)...)
		config[name] = value.ValueString()
	}
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := d.read(ctx, d.client, config)
	if err != nil {
		apiError(&resp.Diagnostics, "look the "+d.name+" up", err)
		return
	}
	if found == nil {
		resp.Diagnostics.AddError("Nothing found", "No "+d.name+" matches what was asked for.")
		return
	}
	for name, value := range config {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), value)...)
	}
	for name, value := range found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), value)...)
	}
}

func key(about string) dsschema.Attribute {
	return dsschema.StringAttribute{MarkdownDescription: about, Required: true}
}

func newProjectDataSource() datasource.DataSource {
	return &lookup{
		name:  "project",
		about: "A project by its key prefix, in any case — `NU` never finds `NUC`.",
		keys:  map[string]dsschema.Attribute{"key_prefix": key("What its task keys lead with.")},
		fields: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{Computed: true},
			"title":       dsschema.StringAttribute{Computed: true},
			"description": dsschema.StringAttribute{Computed: true},
			"currency":    dsschema.StringAttribute{Computed: true},
			"is_active":   dsschema.BoolAttribute{Computed: true},
		},
		read: func(ctx context.Context, c *nuclock.Client, config map[string]string) (map[string]any, error) {
			p, err := c.ProjectByKeyPrefix(ctx, config["key_prefix"])
			if err != nil || p == nil {
				return nil, err
			}
			return map[string]any{"id": p.ID, "title": p.Title, "description": p.Description,
				"currency": p.Currency, "is_active": p.IsActive}, nil
		},
	}
}

func newUserDataSource() datasource.DataSource {
	return &lookup{
		name:  "user",
		about: "A person by their email, whole and in any case.",
		keys:  map[string]dsschema.Attribute{"email": key("Their address.")},
		fields: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{Computed: true},
			"public_name": dsschema.StringAttribute{Computed: true},
			"is_active":   dsschema.BoolAttribute{Computed: true},
		},
		read: func(ctx context.Context, c *nuclock.Client, config map[string]string) (map[string]any, error) {
			u, err := c.UserByEmail(ctx, config["email"])
			if err != nil || u == nil {
				return nil, err
			}
			return map[string]any{"id": u.ID, "public_name": u.PublicName, "is_active": u.IsActive}, nil
		},
	}
}

func newSeatDataSource() datasource.DataSource {
	return &lookup{
		name:  "seat",
		about: "The seat a person holds on a project, open or closed. Its rates are not read.",
		keys: map[string]dsschema.Attribute{
			"project_id": key("The project."),
			"user_id":    key("The person."),
		},
		fields: map[string]dsschema.Attribute{
			"id":          dsschema.StringAttribute{Computed: true},
			"position":    dsschema.StringAttribute{Computed: true},
			"work_format": dsschema.StringAttribute{Computed: true},
			"closed":      dsschema.BoolAttribute{Computed: true},
		},
		read: func(ctx context.Context, c *nuclock.Client, config map[string]string) (map[string]any, error) {
			s, err := c.SeatOf(ctx, config["project_id"], config["user_id"])
			if err != nil || s == nil {
				return nil, err
			}
			return map[string]any{"id": s.ID, "position": s.Position, "work_format": s.WorkFormat,
				"closed": s.ClosedAt != nil}, nil
		},
	}
}

// --- list resources: what `terraform query` finds that nobody manages yet ----

type lister struct {
	client *nuclock.Client
	name   string
	config listschema.Schema
	each   func(ctx context.Context, c *nuclock.Client, req list.ListRequest) ([]listed, error)
}

// listed is one row: its identity, its resource state, and a name to show.
type listed struct {
	display  string
	identity any
	state    any
}

func (l *lister) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + l.name
}

func (l *lister) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = l.config
}

func (l *lister) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	l.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (l *lister) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	rows, err := l.each(ctx, l.client, req)
	if err != nil {
		var diags = list.ListResult{}.Diagnostics
		apiError(&diags, "list "+l.name+"s", err)
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}
	stream.Results = func(yield func(list.ListResult) bool) {
		for count, row := range rows {
			if req.Limit > 0 && int64(count) >= req.Limit {
				return
			}
			result := req.NewListResult(ctx)
			result.DisplayName = row.display
			result.Diagnostics.Append(result.Identity.Set(ctx, row.identity)...)
			if req.IncludeResource {
				result.Diagnostics.Append(result.Resource.Set(ctx, row.state)...)
			}
			if !yield(result) {
				return
			}
		}
	}
}

func newProjectListResource() list.ListResource {
	return &lister{
		name:   "project",
		config: listschema.Schema{MarkdownDescription: "Every live project."},
		each: func(ctx context.Context, c *nuclock.Client, _ list.ListRequest) ([]listed, error) {
			projects, err := c.ListProjects(ctx)
			var rows []listed
			for i := range projects {
				p := &projects[i]
				if !p.IsActive {
					continue
				}
				rows = append(rows, listed{
					display:  p.KeyPrefix + " — " + p.Title,
					identity: projectIdentity{KeyPrefix: types.StringValue(p.KeyPrefix)},
					state:    projectState(p),
				})
			}
			return rows, err
		},
	}
}

func newUserListResource() list.ListResource {
	return &lister{
		name:   "user",
		config: listschema.Schema{MarkdownDescription: "Every active member of the organisation."},
		each: func(ctx context.Context, c *nuclock.Client, _ list.ListRequest) ([]listed, error) {
			users, err := c.ListUsers(ctx)
			var rows []listed
			for i := range users {
				u := &users[i]
				if !u.IsActive {
					continue
				}
				rows = append(rows, listed{
					display:  u.Email,
					identity: userIdentity{Email: types.StringValue(u.Email)},
					state:    userState(u),
				})
			}
			return rows, err
		},
	}
}

func newSeatListResource() list.ListResource {
	return &lister{
		name: "seat",
		config: listschema.Schema{
			MarkdownDescription: "Every open seat, or one project's.",
			Attributes: map[string]listschema.Attribute{
				"project_id": listschema.StringAttribute{Optional: true},
			},
		},
		each: func(ctx context.Context, c *nuclock.Client, req list.ListRequest) ([]listed, error) {
			var projectID types.String
			req.Config.GetAttribute(ctx, path.Root("project_id"), &projectID)
			seats, err := c.ListSeats(ctx, projectID.ValueString())
			var rows []listed
			for i := range seats {
				s := &seats[i]
				if s.ClosedAt != nil {
					continue
				}
				rows = append(rows, listed{
					display:  s.UserID + " @ " + s.ProjectID,
					identity: seatIdentity{ProjectID: types.StringValue(s.ProjectID), UserID: types.StringValue(s.UserID)},
					state:    seatState(s),
				})
			}
			return rows, err
		},
	}
}
