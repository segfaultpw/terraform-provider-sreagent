resource "sreagent_deploy_policy" "checkout" {
  service                = "checkout"
  error_budget_threshold = 10
  incident_block_enabled = true
}
