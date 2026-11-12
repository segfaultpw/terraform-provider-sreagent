# Written by hand from the platform plan for the export's 0.2.0 floor (section P2 of
# docs/superpowers/plans/2026-11-12-terraform-provider-contract-and-release-e.md in sre-agent), not copied from
# the platform: the platform has not written this file yet. It states the shape the provider promises to accept,
# and the test runs it through the same checks as the platform's own golden export. Replace it with the platform's
# golden file once the commit that moves the export's floor to 0.2.0 merges, and delete this one.

terraform {
  required_version = ">= 1.12"
  required_providers {
    sreagent = {
      source  = "segfaultpw/sreagent"
      version = ">= 0.2.0"
    }
  }
}

provider "sreagent" {
  organization = "acme"
}

# A repository that sets only its runner: every setting it does not set follows the organization default.
resource "sreagent_repo_setting" "acme_web" {
  repo            = "acme/web"
  fix_runner      = "self_hosted"
  follows_default = ["platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}

import {
  to = sreagent_repo_setting.acme_web
  id = "00000000-0000-4000-8000-000000000001"
}

# A branch that sets nothing at all.
resource "sreagent_repo_setting" "acme_web_staging" {
  repo            = "acme/web"
  branch          = "staging"
  follows_default = ["fix_runner", "platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}

import {
  to = sreagent_repo_setting.acme_web_staging
  id = "00000000-0000-4000-8000-000000000002"
}

# The organization's own defaults, with no draft_prs (null) and no confirm: the row is already on.
resource "sreagent_repository_defaults" "repository_defaults" {
  fix_runner        = "platform"
  platform_fallback = true
  auto_dispatch     = false
  check_network     = false
  allow_ci_config   = false
}

import {
  to = sreagent_repository_defaults.repository_defaults
  id = "repository_defaults"
}
