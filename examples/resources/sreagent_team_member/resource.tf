resource "sreagent_team" "platform" {
  name = "platform"
}

# One resource per membership: Terraform never removes somebody added in the app.
resource "sreagent_team_member" "ana" {
  team_id = sreagent_team.platform.id
  email   = "ana@example.com"
}
