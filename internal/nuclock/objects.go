package nuclock

import (
	"context"
	"net/http"
	"net/url"
)

// Project is a nuclock project as the provider manages it. Money — the fee,
// the default sale price — is deliberately absent: rates stay in the tracker.
type Project struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	KeyPrefix   string   `json:"key_prefix"`
	Currency    string   `json:"currency"`
	ManagerIDs  []string `json:"manager_ids"`
	IsActive    bool     `json:"is_active"`
}

// User is a member of the organisation. What a drift check compares is here;
// a password, a secret or a recovery code never is — the API sends none.
type User struct {
	ID                      string   `json:"id"`
	Email                   string   `json:"email"`
	Username                *string  `json:"username"`
	PublicName              string   `json:"public_name"`
	IsActive                bool     `json:"is_active"`
	Roles                   []string `json:"roles"`
	AreaIDs                 []string `json:"area_ids"`
	HasSecondFactor         bool     `json:"has_2fa"`
	MustChangePassword      bool     `json:"must_change_password"`
	MustSetupSecondFactor   bool     `json:"must_setup_2fa"`
	PasswordVersion         int64    `json:"password_version"`
	PasswordSetAt           *string  `json:"password_set_at"`
	SecondFactorConfirmedAt *string  `json:"second_factor_confirmed_at"`
	InvitationSentAt        *string  `json:"invitation_sent_at"`
}

// Seat is one person's place on one project. Its rates are never read into
// this struct, so they can never reach a state file.
type Seat struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"project_id"`
	UserID     string   `json:"user_id"`
	Position   string   `json:"position"`
	WorkFormat string   `json:"work_format"`
	AreaIDs    []string `json:"area_ids"`
	CuratorID  *string  `json:"curator_id"`
	ClosedAt   *string  `json:"closed_at"`
}

// Integration is one project's connection to a tracker or a forge. Its
// secrets are write-only: the API says whether each is set, never what it is.
type Integration struct {
	ID                string `json:"id"`
	ProjectID         string `json:"project_id"`
	Kind              string `json:"kind"`
	BaseURL           string `json:"base_url"`
	ExternalProjectID string `json:"external_project_id"`
	IsActive          bool   `json:"is_active"`
	Account           string `json:"account"`
	HasAPIToken       bool   `json:"has_api_token"`
	HasWebhookSecret  bool   `json:"has_webhook_secret"`
}

// --- projects -------------------------------------------------------------------

func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	return &p, c.do(ctx, http.MethodGet, "/v1/project/projects/"+url.PathEscape(id), nil, nil, &p)
}

// ProjectByKeyPrefix is the project whose task keys lead with prefix, in any case.
func (c *Client) ProjectByKeyPrefix(ctx context.Context, prefix string) (*Project, error) {
	rows, err := listAll[Project](ctx, c, "/v1/project/projects", url.Values{"key_prefix": {prefix}})
	return one(rows), err
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	return listAll[Project](ctx, c, "/v1/project/projects", nil)
}

func (c *Client) CreateProject(ctx context.Context, body map[string]any) (*Project, error) {
	var p Project
	return &p, c.do(ctx, http.MethodPost, "/v1/project/projects", nil, body, &p)
}

func (c *Client) UpdateProject(ctx context.Context, id string, body map[string]any) (*Project, error) {
	var p Project
	return &p, c.do(ctx, http.MethodPatch, "/v1/project/projects/"+url.PathEscape(id), nil, body, &p)
}

// --- people -----------------------------------------------------------------------

func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	return &u, c.do(ctx, http.MethodGet, "/v1/user/"+url.PathEscape(id), nil, nil, &u)
}

// UserByEmail is the one person with this address, in any case.
func (c *Client) UserByEmail(ctx context.Context, email string) (*User, error) {
	rows, err := listAll[User](ctx, c, "/v1/user", url.Values{"email": {email}})
	return one(rows), err
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	return listAll[User](ctx, c, "/v1/user", nil)
}

func (c *Client) CreateUser(ctx context.Context, body map[string]any) (*User, error) {
	var u User
	return &u, c.do(ctx, http.MethodPost, "/v1/user", nil, body, &u)
}

func (c *Client) UpdateUser(ctx context.Context, id string, body map[string]any) (*User, error) {
	var u User
	return &u, c.do(ctx, http.MethodPatch, "/v1/user/"+url.PathEscape(id), nil, body, &u)
}

// ReissuePassword mints a new password and mails it to the person. Nothing is
// typed and nothing is echoed: the letter is the only place it exists.
func (c *Client) ReissuePassword(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/v1/user/"+url.PathEscape(id)+"/password", nil, nil, nil)
}

// ResetSecondFactor clears the person's authenticator.
func (c *Client) ResetSecondFactor(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/user/"+url.PathEscape(id)+"/2fa", nil, nil, nil)
}

// ResendInvitation sends the person's invitation letter again: a member's, or a client's.
func (c *Client) ResendInvitation(ctx context.Context, id string, client bool) error {
	which := "/invitation"
	if client {
		which = "/client-invitation"
	}
	return c.do(ctx, http.MethodPost, "/v1/user/"+url.PathEscape(id)+which, nil, nil, nil)
}

// --- seats ------------------------------------------------------------------------

func (c *Client) GetSeat(ctx context.Context, id string) (*Seat, error) {
	var s Seat
	return &s, c.do(ctx, http.MethodGet, "/v1/project/performers/"+url.PathEscape(id), nil, nil, &s)
}

// SeatOf is the one seat this person holds on this project, closed or open.
func (c *Client) SeatOf(ctx context.Context, projectID, userID string) (*Seat, error) {
	rows, err := listAll[Seat](ctx, c, "/v1/project/performers", url.Values{"project_id": {projectID}, "user_id": {userID}})
	return one(rows), err
}

// ListSeats lists every seat, or one project's when projectID is not empty.
func (c *Client) ListSeats(ctx context.Context, projectID string) ([]Seat, error) {
	query := url.Values{}
	if projectID != "" {
		query.Set("project_id", projectID)
	}
	return listAll[Seat](ctx, c, "/v1/project/performers", query)
}

func (c *Client) CreateSeat(ctx context.Context, body map[string]any) (*Seat, error) {
	var s Seat
	return &s, c.do(ctx, http.MethodPost, "/v1/project/performers", nil, body, &s)
}

func (c *Client) UpdateSeat(ctx context.Context, id string, body map[string]any) (*Seat, error) {
	var s Seat
	return &s, c.do(ctx, http.MethodPatch, "/v1/project/performers/"+url.PathEscape(id), nil, body, &s)
}

// CloseSeat ends a seat without deleting what was done on it.
func (c *Client) CloseSeat(ctx context.Context, id string) (*Seat, error) {
	var s Seat
	return &s, c.do(ctx, http.MethodPost, "/v1/project/performers/"+url.PathEscape(id)+"/close", nil, map[string]any{}, &s)
}

func (c *Client) ReopenSeat(ctx context.Context, id string) (*Seat, error) {
	var s Seat
	return &s, c.do(ctx, http.MethodPost, "/v1/project/performers/"+url.PathEscape(id)+"/reopen", nil, map[string]any{}, &s)
}

// --- integrations -----------------------------------------------------------------

func (c *Client) GetIntegration(ctx context.Context, id string) (*Integration, error) {
	var i Integration
	return &i, c.do(ctx, http.MethodGet, "/v1/project/integrations/"+url.PathEscape(id), nil, nil, &i)
}

// IntegrationOf is the one connection of this kind from this project to this target.
func (c *Client) IntegrationOf(ctx context.Context, projectID, kind, target string) (*Integration, error) {
	var rows []Integration
	query := url.Values{"project_id": {projectID}, "kind": {kind}, "external_project_id": {target}}
	err := c.do(ctx, http.MethodGet, "/v1/project/integrations", query, nil, &rows)
	return one(rows), err
}

func (c *Client) CreateIntegration(ctx context.Context, body map[string]any) (*Integration, error) {
	var i Integration
	return &i, c.do(ctx, http.MethodPost, "/v1/project/integrations", nil, body, &i)
}

func (c *Client) UpdateIntegration(ctx context.Context, id string, body map[string]any) (*Integration, error) {
	var i Integration
	return &i, c.do(ctx, http.MethodPatch, "/v1/project/integrations/"+url.PathEscape(id), nil, body, &i)
}

func (c *Client) DeleteIntegration(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/project/integrations/"+url.PathEscape(id), nil, nil, nil)
}
