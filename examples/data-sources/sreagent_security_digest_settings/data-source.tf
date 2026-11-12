data "sreagent_security_digest_settings" "this" {}

output "security_digest_goes_to" {
  value = data.sreagent_security_digest_settings.this.slack_channel
}
