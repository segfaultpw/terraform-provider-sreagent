# In the parent organization: what the child organization acme-labs may use of
# the parent's GitHub account acme. Both must already exist (a platform
# administrator links the child, and the account is connected in the app): the
# resource adopts the row, writes only what is stated here, and leaves it in
# place when destroyed.
#
# A change that takes repositories away from the child (here, going from all to
# two repositories) is applied only against a preview: terraform plan asks the
# platform for it and warns with what the child stops reaching and what is held
# on those repositories, and terraform apply sends the plan's fingerprint.
resource "sreagent_github_child_access" "acme_labs_acme" {
  child_organization = "acme-labs"
  account            = "acme"
  mode               = "selected"
  repositories       = ["acme/web", "acme/infra"]

  # The child's setting, not the account's: state it on one block of the child only.
  context_repo_shared = true
}

# Giving a child everything of another account of the parent, or nothing.
resource "sreagent_github_child_access" "acme_labs_jane" {
  child_organization = "acme-labs"
  account            = "jane"
  mode               = "none"
}
