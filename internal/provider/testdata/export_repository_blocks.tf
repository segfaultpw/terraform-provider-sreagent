# Rendered by the platform's own exporter, not written by hand: SreCore.MCP.HclExport.render/2 at sre-agent commit 2bd5268cf
# (master, v0.487.0, `MIX_ENV=test mix run --no-start`), given one repository, acme/web, with an all-branches entry that
# holds the runner, a staging branch entry that holds nothing, and the organization's own defaults. The platform's golden file
# has a single entry per repository, so this is the file that shows what the export writes when a repository has several:
# fix_runner, platform_fallback and auto_dispatch belong to the whole repository, so they are stated, or named in
# follows_default, on the all-branches block only, and the other blocks neither state nor name them (3207f8c1f). Since re-rendered at
# sre-agent commit 549f97e2a only for the version pin, which now reads >= 0.3.0; nothing else changed (the file never carried the export's header comments).

terraform {
  required_version = ">= 1.12"
  required_providers {
    sreagent = {
      source  = "segfaultpw/sreagent"
      version = ">= 0.3.0"
    }
  }
}

provider "sreagent" {
  organization = "acme"
}

resource "sreagent_repo_setting" "acme_web_all" {
  repo                = "acme/web"
  writable_extensions = []
  fix_runner          = "self_hosted"
  follows_default     = ["platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"]
}

import {
  to = sreagent_repo_setting.acme_web_all
  id = "00000000-0000-4000-8000-000000000001"
}

resource "sreagent_repo_setting" "acme_web_staging" {
  repo                = "acme/web"
  branch              = "staging"
  writable_extensions = []
  follows_default     = ["draft_prs", "check_network", "allow_ci_config"]
}

import {
  to = sreagent_repo_setting.acme_web_staging
  id = "00000000-0000-4000-8000-000000000002"
}

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
