data "sreagent_team_members" "all" {}

output "team_members_count" {
  value = length(data.sreagent_team_members.all.items)
}
