variable "gitlab_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "nuclock_integration" "apollo_repo" {
  project_id          = nuclock_project.apollo.id
  kind                = "gitlab"
  base_url            = "https://gitlab.com"
  external_project_id = "acme/apollo"

  # Never stored in the state. Bump the version to send a rotated token.
  api_token_wo         = var.gitlab_token
  api_token_wo_version = 1
}
