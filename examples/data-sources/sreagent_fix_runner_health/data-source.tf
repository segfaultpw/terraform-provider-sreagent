# Read only: whether the organization's self-hosted fix runner works. Resume,
# Discard and the holding switch are on the runner card in the app. Any key the
# configuration API accepts reads it (api:admin, or a plan-only api:config_read
# key), and the self-hosted runner must be in the plan. last_refusal, with the AI
# provider's own words, is stored in the Terraform state.
data "sreagent_fix_runner_health" "this" {}

output "fix_runner_state" {
  value = data.sreagent_fix_runner_health.this.state
}

output "fix_requests_waiting_for_the_runner" {
  value = data.sreagent_fix_runner_health.this.waiting
}
