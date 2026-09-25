data "sreagent_teams" "all" {}

output "teams_count" {
  value = length(data.sreagent_teams.all.items)
}
