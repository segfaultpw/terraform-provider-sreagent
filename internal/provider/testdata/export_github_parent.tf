resource "sreagent_github_settings" "github_settings" {
  auto_pr_enabled                  = false
  auto_revise_on_review            = false
  draft_prs                        = true
  review_enabled                   = false
  link_comments_enabled            = true
  auto_merge_requires_verification = false
  review_severity_floor            = "medium"
  context_repo                     = "acme/context"
  # This organization has 2 GitHub accounts connected; this block manages acme, the one connected longest. acme-two is managed by its own sreagent_github_account block.
}

import {
  to = sreagent_github_settings.github_settings
  id = "github_settings"
}

resource "sreagent_github_account" "acme_two" {
  account                          = "acme-two"
  auto_pr_enabled                  = false
  auto_revise_on_review            = false
  draft_prs                        = true
  review_enabled                   = false
  link_comments_enabled            = true
  auto_merge_requires_verification = false
  review_severity_floor            = "medium"
}

import {
  to = sreagent_github_account.acme_two
  id = "acme-two"
}

resource "sreagent_github_child_access" "test_org_6290_84d046_acme" {
  child_organization  = "c4985bde-3450-4d2e-856e-2771a17e81e3"
  account             = "acme"
  mode                = "selected"
  repositories        = ["acme/infra"]
  context_repo_shared = true
  # child organization: Test Org 6290-84d046
}

import {
  to = sreagent_github_child_access.test_org_6290_84d046_acme
  id = "c4985bde-3450-4d2e-856e-2771a17e81e3?account=acme"
}

resource "sreagent_github_child_access" "test_org_6290_84d046_acme_two" {
  child_organization = "c4985bde-3450-4d2e-856e-2771a17e81e3"
  account            = "acme-two"
  mode               = "all"
  # child organization: Test Org 6290-84d046
}

import {
  to = sreagent_github_child_access.test_org_6290_84d046_acme_two
  id = "c4985bde-3450-4d2e-856e-2771a17e81e3?account=acme-two"
}

resource "sreagent_github_child_access" "test_org_6306_31eb77_acme" {
  child_organization  = "76ff7e32-e09a-4d0a-adfd-f65f0f6a2944"
  account             = "acme"
  mode                = "none"
  context_repo_shared = true
  # child organization: Test Org 6306-31eb77
}

import {
  to = sreagent_github_child_access.test_org_6306_31eb77_acme
  id = "76ff7e32-e09a-4d0a-adfd-f65f0f6a2944?account=acme"
}

resource "sreagent_github_child_access" "test_org_6306_31eb77_acme_two" {
  child_organization = "76ff7e32-e09a-4d0a-adfd-f65f0f6a2944"
  account            = "acme-two"
  mode               = "all"
  # child organization: Test Org 6306-31eb77
}

import {
  to = sreagent_github_child_access.test_org_6306_31eb77_acme_two
  id = "76ff7e32-e09a-4d0a-adfd-f65f0f6a2944?account=acme-two"
}
