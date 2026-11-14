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

  follows_default = ["check_network", "allow_ci_config", "draft_prs", "auto_merge"]
}

# Fix pull requests that merge by themselves once the checks the branch
# requires pass. Off unless set. Turning it on needs confirm for the apply that
# turns it on: remove confirm afterwards, because a confirmation left in the
# configuration also confirms every later apply that turns it on again.
resource "sreagent_repo_setting" "api_release" {
  repo   = "acme/api"
  branch = "release"

  auto_merge = true
  confirm    = ["auto_merge"]
}
