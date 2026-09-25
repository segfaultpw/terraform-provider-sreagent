# Needs a connected GitHub App installation.
resource "sreagent_github_settings" "this" {
  draft_prs             = true
  review_enabled        = true
  review_severity_floor = "high"
}
