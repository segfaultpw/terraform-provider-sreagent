variable "jira_api_token" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "sreagent_ticket_integration" "jira" {
  provider_type        = "jira"
  base_url             = "https://acme.atlassian.net"
  account_email        = "ops@example.com"
  project_key          = "OPS"
  api_token_wo         = var.jira_api_token
  api_token_wo_version = 1
}
