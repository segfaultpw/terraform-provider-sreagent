# A weekly digest on Monday morning, to a Slack channel and by email.
resource "sreagent_security_digest_settings" "this" {
  enabled       = true
  cadence       = "weekly"
  weekday       = 1
  hour          = 8
  slack_channel = "#security"
  email         = true
}
