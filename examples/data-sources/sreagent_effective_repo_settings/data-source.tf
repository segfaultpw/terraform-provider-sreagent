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

# With more than one GitHub account connected, each repository says which
# account it is read through (null when no account reaches it).
output "repositories_by_account" {
  value = { for r in data.sreagent_effective_repo_settings.self_hosted.items : r.repo => r.account }
}
