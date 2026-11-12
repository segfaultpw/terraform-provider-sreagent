# Needs the organization's GitHub App installed on the repository.
resource "sreagent_repo_setting" "api_main" {
  repo    = "acme/api"
  branch  = "main"
  service = "api"

  # Several directories a fix may change; use path_prefix for exactly one.
  path_prefixes = ["services/api/", "services/web/"]

  # Open fix pull requests ready for review on this branch.
  draft_prs = false
}

# A setting a configuration leaves out is left as it is. To put a setting back
# on the organization's repository defaults, remove it and name it in
# follows_default; set_here and following say which settings a row holds.
resource "sreagent_repo_setting" "api_staging" {
  repo   = "acme/api"
  branch = "staging"

  follows_default = ["check_network", "allow_ci_config", "draft_prs"]
}
