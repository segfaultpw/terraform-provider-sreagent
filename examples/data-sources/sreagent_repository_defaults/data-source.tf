# configured is false for a child organization that follows its parent's defaults.
data "sreagent_repository_defaults" "this" {}

output "fix_runner" {
  value = data.sreagent_repository_defaults.this.fix_runner
}
