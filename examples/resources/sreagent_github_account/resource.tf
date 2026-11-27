# The settings of one GitHub account the organization connected. The account
# must already be connected (Integrations, GitHub): the resource adopts it,
# changes only what is stated here, and leaves it connected when destroyed.
#
# sreagent_github_settings manages the organization's managed account (the one
# connected longest) and the context repository. Use this resource for the
# organization's other accounts; a block for the managed account fights it.
resource "sreagent_github_account" "acme_labs" {
  account               = "acme-labs"
  review_enabled        = true
  review_severity_floor = "high"
  draft_prs             = true
}
