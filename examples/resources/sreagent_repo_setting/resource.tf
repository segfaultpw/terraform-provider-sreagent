# Needs the organization's GitHub App installed on the repository.
resource "sreagent_repo_setting" "api_main" {
  repo        = "acme/api"
  branch      = "main"
  service     = "api"
  path_prefix = "services/api"
}
