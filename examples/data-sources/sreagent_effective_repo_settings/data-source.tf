# What every repository runs with, and whether each value follows the
# organization's repository defaults. Every page is read.
data "sreagent_effective_repo_settings" "self_hosted" {
  runner = "self_hosted"
}

output "repositories_on_your_runner" {
  value = [for r in data.sreagent_effective_repo_settings.self_hosted.items : r.repo]
}

output "defaults" {
  value = jsondecode(data.sreagent_effective_repo_settings.self_hosted.defaults)
}
