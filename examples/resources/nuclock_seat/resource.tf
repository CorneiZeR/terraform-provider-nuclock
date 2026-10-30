# Puts Ann on Apollo. Removing this block closes the seat; her hours stay.
resource "nuclock_seat" "ann_on_apollo" {
  project_id  = nuclock_project.apollo.id
  user_id     = nuclock_user.ann.id
  position    = "Backend developer"
  work_format = "hour"
}
