# In the parent organization: one row for each child and account, with what the
# child may use and whether it applies now.
data "sreagent_github_child_access" "all" {}

output "children_that_read_nothing" {
  value = [for r in data.sreagent_github_child_access.all.items : "${r.child_name} / ${r.account}" if !r.applies]
}
