resource "sreagent_github_settings" "github_settings" {
  auto_pr_enabled                  = false
  auto_revise_on_review            = false
  draft_prs                        = true
  review_enabled                   = false
  link_comments_enabled            = true
  auto_merge_requires_verification = false
  review_severity_floor            = "medium"
  # This organization has 2 GitHub accounts connected; this block manages jane, the one connected longest. janet is managed by its own sreagent_github_account block.
}

import {
  to = sreagent_github_settings.github_settings
  id = "github_settings"
}

resource "sreagent_github_account" "janet" {
  account                          = "janet"
  auto_pr_enabled                  = false
  auto_revise_on_review            = false
  draft_prs                        = true
  review_enabled                   = false
  link_comments_enabled            = true
  auto_merge_requires_verification = false
  review_severity_floor            = "medium"
}

import {
  to = sreagent_github_account.janet
  id = "janet"
}
