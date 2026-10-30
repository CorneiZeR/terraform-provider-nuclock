# Creating a person mails them a password they must change at the first sign-in.
resource "nuclock_user" "ann" {
  email       = "ann@example.com"
  public_name = "Ann Lee"
  roles       = ["executor"]
}
