---
page_title: "sreagent_github_account Resource - sreagent"
subcategory: ""
description: |-
  The settings of one GitHub account the organization has connected: open fix pull requests automatically, revise a pull request when a review asks, open them as drafts, review pull requests, comment links on linked issues, "Auto-merge only what SRE Agent verified" and the lowest severity a review comments on. It is the per-account form of sreagent_github_settings.
  Which one to use: sreagent_github_settings manages the organization's managed account, the one connected longest, and the organization's context repository. Use sreagent_github_account for any other account of an organization that connected more than one. A block for the managed account fights sreagent_github_settings (both write the same row), so the plan warns when the account it adopts is the managed one, and the platform's exported configuration writes this resource only for the accounts that are not the managed one. The managed account is not fixed: it is the oldest account the organization has connected, so when that account is disconnected the next oldest becomes the managed one. A block for that next account, which did not overlap sreagent_github_settings before, now does (the plan warns on it from the next refresh): then manage that account with sreagent_github_settings and remove the block. An organization with one account manages it with sreagent_github_settings.
  The platform has no door that connects or disconnects an account, so the resource adopts one that is connected: creating it reads the account by its login and writes only the settings the block states and the account does not already hold, and a setting the block leaves out is left as it is. An account that is not connected is an error that says where to connect it (the Integrations page). Destroying the resource leaves the account connected and its settings in place; disconnect it on the Integrations page. The context repository is not an attribute here: it belongs to the organization, and sreagent_github_settings sets it.
  Import it with the account's login as the platform shows it (terraform import sreagent_github_account.NAME acme-labs). A GitHub account that is renamed on GitHub changes its login, so after a rename the block is edited to the new login and imported again. An account the organization reads through its parent (a child organization) is not its own row: its settings are the parent organization's, and this resource cannot adopt it. Needs the GitHub integration in the plan and an api:admin key; a plan-only api:config_read key plans and cannot apply.
---

# sreagent_github_account (Resource)

The settings of one GitHub account the organization has connected: open fix pull requests automatically, revise a pull request when a review asks, open them as drafts, review pull requests, comment links on linked issues, "Auto-merge only what SRE Agent verified" and the lowest severity a review comments on. It is the per-account form of sreagent_github_settings.

Which one to use: sreagent_github_settings manages the organization's managed account, the one connected longest, and the organization's context repository. Use sreagent_github_account for any other account of an organization that connected more than one. A block for the managed account fights sreagent_github_settings (both write the same row), so the plan warns when the account it adopts is the managed one, and the platform's exported configuration writes this resource only for the accounts that are not the managed one. The managed account is not fixed: it is the oldest account the organization has connected, so when that account is disconnected the next oldest becomes the managed one. A block for that next account, which did not overlap sreagent_github_settings before, now does (the plan warns on it from the next refresh): then manage that account with sreagent_github_settings and remove the block. An organization with one account manages it with sreagent_github_settings.

The platform has no door that connects or disconnects an account, so the resource adopts one that is connected: creating it reads the account by its login and writes only the settings the block states and the account does not already hold, and a setting the block leaves out is left as it is. An account that is not connected is an error that says where to connect it (the Integrations page). Destroying the resource leaves the account connected and its settings in place; disconnect it on the Integrations page. The context repository is not an attribute here: it belongs to the organization, and sreagent_github_settings sets it.

Import it with the account's login as the platform shows it (`terraform import sreagent_github_account.NAME acme-labs`). A GitHub account that is renamed on GitHub changes its login, so after a rename the block is edited to the new login and imported again. An account the organization reads through its parent (a child organization) is not its own row: its settings are the parent organization's, and this resource cannot adopt it. Needs the GitHub integration in the plan and an api:admin key; a plan-only api:config_read key plans and cannot apply.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `account` (String) The account's login on GitHub, as the Integrations page shows it (the platform ignores case). Changing it replaces the resource: the old account keeps its settings and the block adopts the other one.

### Optional

- `auto_merge_requires_verification` (Boolean) Auto-merge only what SRE Agent verified. Off by default: a fix SRE Agent's own checks did not run for can still merge by itself once the checks its branch requires pass (where a repository's auto_merge says so), and a change pushed onto a pull request afterwards merges on those checks alone. An organization admin takes that risk by leaving it off. On: only a fix whose exact commit those checks ran on and passed merges by itself. In both positions a fix whose checks failed, or ran and could not judge it, or has an open self-review blocker, never merges by itself.
- `auto_pr_enabled` (Boolean) Open fix pull requests automatically.
- `auto_revise_on_review` (Boolean) Revise a pull request when a review asks.
- `draft_prs` (Boolean) Open fix pull requests as drafts. It is this account's choice; a repository's own draft_prs (sreagent_repo_setting) and the organization default (sreagent_repository_defaults) are read first. Once set, Terraform cannot return it to null; do that in the app.
- `link_comments_enabled` (Boolean) Comment links on linked issues.
- `review_enabled` (Boolean) Review pull requests.
- `review_severity_floor` (String) The lowest severity a review comments on. One of: critical, high, medium, low.

### Read-Only

- `account_id` (Number) GitHub's id of the account, or null when it has not been recorded yet.
- `account_type` (String) User or Organization, as GitHub reports it, or null when it has not been recorded yet.
- `holds_context_repo` (Boolean) True when the organization's context repository is read through this account.
- `id` (String) The row's identifier on the platform.
- `installation_id` (Number) The GitHub App installation id of the account.
- `managed` (Boolean) True for the organization's managed account, the one sreagent_github_settings manages.
- `repos` (String) The repositories the account reaches, at most 500, as the platform names them: a JSON array, decoded with jsondecode.
- `repos_refreshed_at` (String) When the platform last read the account's repository list, or null when it never has.
- `repos_truncated` (Boolean) True when repos holds fewer repositories than the account reaches (the platform answers at most 500).
- `repository_count` (Number) How many repositories the account reaches, as of the last time the platform read the list.
- `status` (String) active, or suspended while GitHub has the installation suspended.

## Import

Import is supported using the following syntax, or with an `import` block
naming the same ID. See the [importing guide](../guides/importing.md).

```shell
# An account imports by its login, as the Integrations page shows it. A GitHub
# account that is renamed changes its login, so edit the block and import it again.
terraform import sreagent_github_account.acme_labs acme-labs
```
