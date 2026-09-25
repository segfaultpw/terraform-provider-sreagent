data "sreagent_repo_settings" "all" {}

output "repo_settings_count" {
  value = length(data.sreagent_repo_settings.all.items)
}
