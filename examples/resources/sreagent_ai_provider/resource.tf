variable "anthropic_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "sreagent_ai_provider" "anthropic" {
  name               = "anthropic"
  provider_type      = "anthropic"
  model              = "claude-sonnet-5"
  api_key_wo         = var.anthropic_api_key
  api_key_wo_version = 1
}
