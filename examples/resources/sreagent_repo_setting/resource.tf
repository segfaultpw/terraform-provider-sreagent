# Needs the organization's GitHub App installed on the repository.
resource "sreagent_repo_setting" "api_main" {
  repo    = "acme/api"
  branch  = "main"
  service = "api"

  # Several directories a fix may change; use path_prefix for exactly one.
  path_prefixes = ["services/api/", "services/web/"]

  # Open fix pull requests ready for review on this branch (leave it out to
  # follow the GitHub installation's draft_prs setting).
  draft_prs = false
}
