# Read only: whether the organization's self-hosted fix runner works. Resume,
# Discard and the holding switch are on the runner card in the app. The read
# needs an organization admin's key and the self-hosted runner in the plan.
data "sreagent_fix_runner_health" "this" {}

output "fix_runner_state" {
  value = data.sreagent_fix_runner_health.this.state
}

output "fix_requests_waiting_for_the_runner" {
  value = data.sreagent_fix_runner_health.this.waiting
}
