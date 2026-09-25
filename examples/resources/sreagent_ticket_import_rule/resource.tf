resource "sreagent_ticket_import_rule" "jira" {
  provider_type = "jira"
  enabled       = true
  config = jsonencode({
    project = "OPS"
  })
}
