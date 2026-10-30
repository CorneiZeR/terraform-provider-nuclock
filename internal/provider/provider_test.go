package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"nuclock": providerserver.NewProtocol6WithError(New("test")()),
}

func config(url, body string) string {
	return fmt.Sprintf("provider \"nuclock\" {\n  url     = %q\n  api_key = \"test-key\"\n}\n%s", url, body)
}

const team = `
resource "nuclock_project" "p" {
  title      = "Apollo"
  key_prefix = "APO"
}
resource "nuclock_user" "ann" {
  email = "ann@example.com"
  roles = ["executor"]
}
`

const seat = `
resource "nuclock_seat" "ann" {
  project_id = nuclock_project.p.id
  user_id    = nuclock_user.ann.id
  position   = "Engineer"
}
`

// The task's "done when": an apply puts a person on a project and takes them
// off it, and a plan on an unchanged configuration is empty.
func TestSeatOnAndOff(t *testing.T) {
	fake, server := newFakeAPI(t)
	var seatID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config(server.URL, team+seat),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("nuclock_seat.ann", "position", "Engineer"),
					func(s *terraform.State) error {
						seatID = s.RootModule().Resources["nuclock_seat.ann"].Primary.ID
						return nil
					},
				),
			},
			// The same configuration again plans nothing (the framework fails the step otherwise).
			{Config: config(server.URL, team+seat), PlanOnly: true},
			{
				// Off the project: the seat CLOSES, it is not deleted.
				Config: config(server.URL, team),
				Check: func(*terraform.State) error {
					if fake.seats[seatID]["closed_at"] == nil {
						return fmt.Errorf("the seat was not closed")
					}
					return nil
				},
			},
			{
				// Back on: the same seat, reopened, its history with it.
				Config: config(server.URL, team+seat),
				Check: resource.ComposeTestCheckFunc(
					func(s *terraform.State) error {
						if got := s.RootModule().Resources["nuclock_seat.ann"].Primary.ID; got != seatID {
							return fmt.Errorf("a second seat %s was made instead of reopening %s", got, seatID)
						}
						if fake.seats[seatID]["closed_at"] != nil {
							return fmt.Errorf("the seat is still closed")
						}
						return nil
					},
				),
			},
		},
	})
	// Rates stay in the tracker: no seat write ever carried one.
	if len(fake.sentPrices) > 0 {
		t.Fatalf("a seat write sent rates: %v", fake.sentPrices)
	}
}

// No secret reaches the state; a new version sends the new token.
func TestIntegrationSecretsAreWriteOnly(t *testing.T) {
	fake, server := newFakeAPI(t)
	integration := func(site, token string, version int) string {
		return team + fmt.Sprintf(`
resource "nuclock_integration" "repo" {
  project_id           = nuclock_project.p.id
  kind                 = "gitlab"
  base_url             = %q
  external_project_id  = "acme/app"
  api_token_wo         = %q
  api_token_wo_version = %d
}
`, site, token, version)
	}
	noSecret := func(s *terraform.State) error {
		for _, r := range s.RootModule().Resources {
			for key, value := range r.Primary.Attributes {
				if strings.Contains(value, "glpat-") {
					return fmt.Errorf("%s.%s holds the secret", r.Type, key)
				}
			}
		}
		return nil
	}
	sent := func(want string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			id := s.RootModule().Resources["nuclock_integration.repo"].Primary.ID
			if got := fake.secrets[id+"/api_token"]; got != want {
				return fmt.Errorf("the server holds %q, want %q", got, want)
			}
			return nil
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config(server.URL, integration("https://gitlab.com", "glpat-first", 1)),
				Check: resource.ComposeTestCheckFunc(noSecret, sent("glpat-first"),
					resource.TestCheckResourceAttr("nuclock_integration.repo", "has_api_token", "true")),
			},
			// A changed value at the SAME version is not sent: the version is the
			// trigger. The address moves too, or no update would run at all — a
			// write-only value alone is never a difference to Terraform.
			{Config: config(server.URL, integration("https://gitlab.example.com", "glpat-second", 1)), Check: sent("glpat-first")},
			{Config: config(server.URL, integration("https://gitlab.example.com", "glpat-second", 2)), Check: resource.ComposeTestCheckFunc(noSecret, sent("glpat-second"))},
		},
	})
}

// A key without the ability is told which one, in the API's own words.
func TestAMissingAbilityIsNamed(t *testing.T) {
	fake, server := newFakeAPI(t)
	fake.refuse = "projects.write"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      config(server.URL, team),
			ExpectError: regexp.MustCompile(`ability_missing[\s\S]*projects\.write`),
		}},
	})
}

// A person declared again after a destroy is let back in, not refused as a twin.
func TestABlockedPersonComesBack(t *testing.T) {
	fake, server := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config(server.URL, team)},
			{
				Config: config(server.URL, `resource "nuclock_project" "p" {
  title      = "Apollo"
  key_prefix = "APO"
}`),
				Check: func(*terraform.State) error {
					for _, u := range fake.users {
						if u["is_active"] != false {
							return fmt.Errorf("the person was not blocked")
						}
					}
					return nil
				},
			},
			{
				Config: config(server.URL, team),
				Check: func(*terraform.State) error {
					if len(fake.users) != 1 {
						return fmt.Errorf("%d accounts, want the one let back in", len(fake.users))
					}
					return nil
				},
			},
		},
	})
}

// A member of staff who is also somebody's client keeps the client's role
// through a staff edit: the API replaces roles, and that one is the product's.
func TestAClientRoleSurvivesAStaffEdit(t *testing.T) {
	fake, server := newFakeAPI(t)
	person := func(name string) string {
		return fmt.Sprintf(`
resource "nuclock_user" "ann" {
  email       = "ann@example.com"
  public_name = %q
  roles       = ["executor"]
}
`, name)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config(server.URL, person("Ann")),
				Check: func(*terraform.State) error {
					// The product makes her a client: she was given a project.
					for _, u := range fake.users {
						u["roles"] = []any{"executor", "client"}
					}
					return nil
				},
			},
			{
				Config: config(server.URL, person("Ann Lee")),
				Check: func(*terraform.State) error {
					for _, u := range fake.users {
						if !strings.Contains(fmt.Sprint(u["roles"]), "client") {
							return fmt.Errorf("the client's role was taken away: %v", u["roles"])
						}
					}
					return nil
				},
			},
		},
	})
}

// A project's owners are the clients named in the configuration, sent whole.
func TestProjectOwners(t *testing.T) {
	fake, server := newFakeAPI(t)
	owned := func(owners string) string {
		return fmt.Sprintf(`
resource "nuclock_project" "p" {
  title      = "Apollo"
  key_prefix = "APO"
  owner_ids  = [%s]
}
`, owners)
	}
	owners := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			for _, p := range fake.projects {
				if got := fmt.Sprint(p["owner_ids"]); got != want {
					return fmt.Errorf("owners %s, want %s", got, want)
				}
			}
			return nil
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config(server.URL, owned(`"client-1"`)), Check: owners("[client-1]")},
			{Config: config(server.URL, owned(`"client-1", "client-2"`)), PlanOnly: false, Check: resource.TestCheckResourceAttr("nuclock_project.p", "owner_ids.#", "2")},
		},
	})
}

// What a configuration leaves out is not managed: an import keeps it, and
// dropping it from the configuration neither clears it nor plans a change.
func TestWhatIsLeftOutIsKept(t *testing.T) {
	fake, server := newFakeAPI(t)
	full := config(server.URL, `
resource "nuclock_project" "p" {
  title       = "Apollo"
  key_prefix  = "APO"
  description = "Kept"
}
resource "nuclock_user" "ann" {
  email       = "ann@example.com"
  public_name = "Ann Lee"
  roles       = ["executor"]
}
resource "nuclock_seat" "ann" {
  project_id  = nuclock_project.p.id
  user_id     = nuclock_user.ann.id
  position    = "Engineer"
  work_format = "full"
}
`)
	bare := config(server.URL, `
resource "nuclock_project" "p" {
  title      = "Apollo"
  key_prefix = "APO"
}
resource "nuclock_user" "ann" {
  email = "ann@example.com"
  roles = ["executor"]
}
resource "nuclock_seat" "ann" {
  project_id = nuclock_project.p.id
  user_id    = nuclock_user.ann.id
}
`)
	kept := func(*terraform.State) error {
		for _, p := range fake.projects {
			if p["description"] != "Kept" {
				return fmt.Errorf("description %v", p["description"])
			}
		}
		for _, u := range fake.users {
			if u["public_name"] != "Ann Lee" {
				return fmt.Errorf("public_name %v", u["public_name"])
			}
		}
		for _, s := range fake.seats {
			if s["position"] != "Engineer" || s["work_format"] != "full" {
				return fmt.Errorf("seat %v / %v", s["position"], s["work_format"])
			}
		}
		return nil
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: full, Check: kept},
			{Config: bare, PlanOnly: true},
			{Config: bare, Check: kept},
		},
	})
}

// An import naming an id that does not exist says so, in words — not the
// framework's "Missing Resource Identity After Read".
func TestImportOfAMissingIDIsNamed(t *testing.T) {
	_, server := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config(server.URL, team+seat)},
			{
				Config:        config(server.URL, team+seat),
				ResourceName:  "nuclock_seat.ann",
				ImportState:   true,
				ImportStateId: "seat-that-is-not-there",
				ExpectError:   regexp.MustCompile(`No such seat[\s\S]*seat-that-is-not-there`),
			},
		},
	})
}
