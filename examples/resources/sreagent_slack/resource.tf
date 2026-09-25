variable "slack_bot_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

variable "slack_signing_secret" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "sreagent_slack" "this" {
  name                      = "acme"
  default_channel_critical  = "#pager"
  bot_token_wo              = var.slack_bot_token
  bot_token_wo_version      = 1
  signing_secret_wo         = var.slack_signing_secret
  signing_secret_wo_version = 1
}
