# terraform apply -invoke=action.nuclock_reissue_password.ann
# A new password is mailed to Ann; nothing here ever holds it.
action "nuclock_reissue_password" "ann" {
  config {
    user_id = nuclock_user.ann.id
  }
}
