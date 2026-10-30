resource "nuclock_project" "apollo" {
  title       = "Apollo"
  description = "The client portal rebuild"
  key_prefix  = "APO"
  currency    = "USD"
  manager_ids = [nuclock_user.lead.id]
}
