data "sreagent_organization_settings" "this" {}

# Who may ask SRE Agent for a fix pull request (org_admin or member). An org
# admin changes it in the app; Terraform only reads it.
output "fix_request_floor" {
  value = data.sreagent_organization_settings.this.fix_request_floor
}

# How reactions travel between a card and its Slack threads (both, from_slack or off).
output "slack_reaction_sync" {
  value = data.sreagent_organization_settings.this.slack_reaction_sync
}
