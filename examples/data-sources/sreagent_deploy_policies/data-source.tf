data "sreagent_deploy_policies" "all" {}

output "deploy_policies_count" {
  value = length(data.sreagent_deploy_policies.all.items)
}
