# Written by hand from the platform plan for the export's 0.3.0 floor (section P2 of
# docs/superpowers/plans/2026-11-13-terraform-provider-v0-3-0.md in sre-agent), not copied from the platform: the platform
# does not write the five security blocks yet (they carry tf_unreleased? until provider 0.3.0 is on the Registry). It states
# the shape the provider promises to accept, and the tests run it through the same checks as the platform's own golden
# export and adopt it with imports alone. Replace it with the security blocks of the platform's golden file once the commit
# that moves the floor to 0.3.0 merges, and delete this one.
#
# The shapes that matter: a day or a channel that is null on the platform is written as its sentinel (info_days = 0,
# slack_channel = ""), a rule states every field it holds (the seeded rule's events differ from the default), an allow rule is
# labelled by its repository and glob and states the instant it ends.

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

resource "sreagent_security_sla_policy" "security_sla_policy" {
  critical_days = 7
  high_days     = 30
  medium_days   = 90
  low_days      = 180
  info_days     = 0
}

import {
  to = sreagent_security_sla_policy.security_sla_policy
  id = "security_sla_policy"
}

resource "sreagent_security_digest_settings" "security_digest_settings" {
  enabled       = false
  cadence       = "weekly"
  weekday       = 1
  hour          = 9
  slack_channel = ""
  email         = false
}

import {
  to = sreagent_security_digest_settings.security_digest_settings
  id = "security_digest_settings"
}

resource "sreagent_security_code_classes" "security_code_classes" {
  classes = ["other:ssrf", "other:xxe"]
}

import {
  to = sreagent_security_code_classes.security_code_classes
  id = "security_code_classes"
}

resource "sreagent_security_notification_rule" "critical_and_high_findings" {
  name         = "Critical and high findings"
  enabled      = true
  areas        = []
  min_severity = "high"
  events       = ["new_finding", "sla_breached", "acceptance_expired", "fix_verified"]
  slack        = true
  email        = true
  page         = false
}

import {
  to = sreagent_security_notification_rule.critical_and_high_findings
  id = "00000000-0000-4000-8000-000000000011"
}

resource "sreagent_security_notification_rule" "page_on_critical" {
  name          = "Page on critical"
  enabled       = true
  areas         = ["secrets", "iam"]
  min_severity  = "critical"
  events        = ["new_finding"]
  slack         = true
  slack_channel = "#security-critical"
  email         = false
  page          = true
}

import {
  to = sreagent_security_notification_rule.page_on_critical
  id = "00000000-0000-4000-8000-000000000012"
}

resource "sreagent_secret_allow_rule" "acme_web_testdata" {
  path_glob     = "**/testdata/**"
  reason        = "Vendored test fixtures, not real keys"
  repository    = "acme/web"
  detector_type = "aws_access_key"
  expires_at    = "2020-01-01T00:00:00.000000Z"
}

import {
  to = sreagent_secret_allow_rule.acme_web_testdata
  id = "00000000-0000-4000-8000-000000000013"
}

resource "sreagent_secret_allow_rule" "all_vendor" {
  path_glob = "vendor/**"
  reason    = "Third party code is scanned upstream"
}

import {
  to = sreagent_secret_allow_rule.all_vendor
  id = "00000000-0000-4000-8000-000000000014"
}
