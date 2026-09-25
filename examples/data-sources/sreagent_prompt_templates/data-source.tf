data "sreagent_prompt_templates" "all" {}

# A capped list says when it may hold more rows than it answered.
output "prompt_templates_truncated" {
  value = data.sreagent_prompt_templates.all.truncated
}
