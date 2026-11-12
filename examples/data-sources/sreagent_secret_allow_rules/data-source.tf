data "sreagent_secret_allow_rules" "all" {}

output "allow_rules_for_all_repositories" {
  value = [for r in data.sreagent_secret_allow_rules.all.items : r.path_glob if r.repository == null]
}
