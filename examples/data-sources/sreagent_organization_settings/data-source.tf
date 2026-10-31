data "sreagent_organization_settings" "this" {}

# Who may ask SRE Agent for a fix pull request (org_admin or member). An org
# admin changes it in the app; Terraform only reads it.
output "fix_request_floor" {
  value = data.sreagent_organization_settings.this.fix_request_floor
}
