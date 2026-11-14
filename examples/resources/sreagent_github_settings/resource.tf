# Needs a connected GitHub App installation.
resource "sreagent_github_settings" "this" {
  draft_prs             = true
  review_enabled        = true
  review_severity_floor = "high"

  # Auto-merge only what SRE Agent verified: a fix whose checks did not run for
  # it never merges by itself, even where a repository's auto_merge is on.
  auto_merge_requires_verification = true
}
