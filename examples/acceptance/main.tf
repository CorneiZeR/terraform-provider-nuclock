# `make testacc`: an apply against a real nuclock, then a plan that must be empty.
# Leaves a project TFA, a person and a seat behind; `terraform destroy` archives,
# blocks and closes them.
terraform {
  required_providers {
    nuclock = { source = "corneizer/nuclock" }
  }
}

provider "nuclock" {}

resource "nuclock_project" "acc" {
  title      = "Terraform acceptance"
  key_prefix = "TFA"
}

resource "nuclock_user" "dev" {
  email = "tf-acceptance@example.com"
  roles = ["executor"]
}

resource "nuclock_integration" "repo" {
  project_id           = nuclock_project.acc.id
  kind                 = "gitlab"
  base_url             = "https://gitlab.com"
  external_project_id  = "acme/tf-acceptance"
  api_token_wo         = "glpat-acceptance-not-a-real-token"
  api_token_wo_version = 1
}
