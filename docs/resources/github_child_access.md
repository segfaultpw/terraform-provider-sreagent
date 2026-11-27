---
page_title: "sreagent_github_child_access Resource - sreagent"
subcategory: ""
description: |-
  What the parent organization gives one child organization of one of the parent's GitHub accounts: all of the account's repositories (mode all), only the ones named (selected, at most 500) or none, and whether the child may use the parent's context repository. One block for each child and account.
  Use it in the parent organization. A child reads only what it is given here: a child linked under the parent is given all of the accounts connected at that moment, except that a child organization that has its own GitHub account is given nothing of its parent's accounts until an admin of the parent gives it repositories here. An account connected later reaches no existing child until it is given. A repository the child is not given is never read, minted a token for or opened a pull request on for the child.
  The platform has no door that creates or deletes the row (a child is linked under its parent by a platform administrator, and the account is connected in the app), so the resource adopts it: creating it reads the row, judges the block against it and writes only the difference, and a setting the block leaves out is left as it is. A child or account that does not exist is an error that says where to link or connect it. Destroying the resource leaves the row in place and changes nothing; the child keeps what it was given, so to take access away state mode = "none" (or narrow it) and apply.
  A change that takes repositories away from the child is applied only against a preview of what it moves, so terraform plan asks the platform for the preview (it needs an api:admin key; with an api:config_read key the plan says so, and the apply previews and applies) and warns with what the child may use now and will use, the repositories it stops reaching, and what is held on them: open fix pull requests stay open on GitHub with one comment saying SRE Agent no longer manages them, auto-merge is turned off, and waiting requests and the cards are named. Nothing is closed or deleted. The apply sends the plan's fingerprint and fails without writing when what would move changed since the plan; plan again. Where the plan could not take the preview (an api:config_read key, or a child or account known only at apply) the apply takes it and applies what it finds, with a warning, so review the plan with an api:admin key first. A change that takes nothing away (giving the child more, or changing only context_repo_shared) is applied at once.
  Import it as CHILD_ID?account=ACCOUNT (terraform import sreagent_github_child_access.NAME CHILD_ID?account=acme); without the account an organization with one connected account takes that one. CHILD_ID is the child's id or its name: write it the way the block writes child_organization, and the account the way the block writes account. A block that names the same row another way (the child's name for its id, another case of the login) is not a new row: the plan shows one in-place update that sends nothing and never a replacement. The key's organization must be the parent. Needs the GitHub integration in the plan.
---

# sreagent_github_child_access (Resource)

What the parent organization gives one child organization of one of the parent's GitHub accounts: all of the account's repositories (mode all), only the ones named (selected, at most 500) or none, and whether the child may use the parent's context repository. One block for each child and account.

Use it in the parent organization. A child reads only what it is given here: a child linked under the parent is given all of the accounts connected at that moment, except that a child organization that has its own GitHub account is given nothing of its parent's accounts until an admin of the parent gives it repositories here. An account connected later reaches no existing child until it is given. A repository the child is not given is never read, minted a token for or opened a pull request on for the child.

The platform has no door that creates or deletes the row (a child is linked under its parent by a platform administrator, and the account is connected in the app), so the resource adopts it: creating it reads the row, judges the block against it and writes only the difference, and a setting the block leaves out is left as it is. A child or account that does not exist is an error that says where to link or connect it. Destroying the resource leaves the row in place and changes nothing; the child keeps what it was given, so to take access away state mode = "none" (or narrow it) and apply.

A change that takes repositories away from the child is applied only against a preview of what it moves, so terraform plan asks the platform for the preview (it needs an api:admin key; with an api:config_read key the plan says so, and the apply previews and applies) and warns with what the child may use now and will use, the repositories it stops reaching, and what is held on them: open fix pull requests stay open on GitHub with one comment saying SRE Agent no longer manages them, auto-merge is turned off, and waiting requests and the cards are named. Nothing is closed or deleted. The apply sends the plan's fingerprint and fails without writing when what would move changed since the plan; plan again. Where the plan could not take the preview (an api:config_read key, or a child or account known only at apply) the apply takes it and applies what it finds, with a warning, so review the plan with an api:admin key first. A change that takes nothing away (giving the child more, or changing only context_repo_shared) is applied at once.

Import it as `CHILD_ID?account=ACCOUNT` (`terraform import sreagent_github_child_access.NAME CHILD_ID?account=acme`); without the account an organization with one connected account takes that one. CHILD_ID is the child's id or its name: write it the way the block writes child_organization, and the account the way the block writes account. A block that names the same row another way (the child's name for its id, another case of the login) is not a new row: the plan shows one in-place update that sends nothing and never a replacement. The key's organization must be the parent. Needs the GitHub integration in the plan.

## Example Usage

```terraform
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `account` (String) The parent's GitHub account, by its login as the Integrations page shows it (the platform ignores case) or its installation number. Changing it replaces the resource.
- `child_organization` (String) The child organization, by its id or its name. Changing it replaces the resource: the old row keeps what it holds.

### Optional

- `context_repo_shared` (Boolean) Whether the child may use the parent's context repository (the one sreagent_github_settings sets), through an account it was given something of. It is the child's setting, not the account's: the platform holds one value for each child whichever account's row states it. State it on one block of the child only, because two blocks of one child that state different values change it on every apply. On by default.
- `mode` (String) What the child may use of the account: all (every repository the account reaches, now and later), selected (only the repositories in repositories) or none. Choosing all or none while repositories lists some is an error. One of: all, selected, none.
- `repositories` (List of String) The repositories a selected child may use, at most 500, as owner/name: the plan refuses a repository the account does not reach, with the platform's sentence. Only with mode = "selected" (the platform answers an empty list for all and none, and the attribute follows the mode). The order and the case do not matter (acme/web and Acme/Web are one repository, and naming both is refused); state keeps the spelling written here, and repository_details shows the platform's. A repository the account connects later is not given until it is named here.

### Read-Only

- `applies` (Boolean) True when the child reads this grant. False when it cannot yet (reason says why).
- `child_name` (String) The child organization's name.
- `details_read` (Boolean) True when the platform has read the account's repository list for a selected child, so repository_details is complete.
- `id` (String) The row's identifier on the platform.
- `installation_id` (Number) The GitHub App installation id of the parent's account.
- `own_github` (Boolean) True when the child holds a GitHub account of its own. The parent never learns which.
- `preview_fingerprint` (String) The fingerprint of the preview the plan took, which the apply sends. Kept by the provider; never configure it.
- `reason` (String) Why the grant does not apply, or null while it does: for example the child stopped using the parent's GitHub, or it connected its own GitHub account and was given nothing at the time.
- `repository_count` (Number) How many repositories of the account the child reaches now.
- `repository_details` (String) For a selected child, each repository it was given with whether the account still reaches it, as the platform answers it, as a JSON array decoded with jsondecode.
- `source` (String) How the row came to be: for example link (made when the child was linked), shipped, own_at_cutoff (the child already had a GitHub account of its own), or a change by a person or a key.
- `stopped` (Boolean) True when the child chose to stop using its parent's GitHub, so none of this applies until it resumes.
- `updated_at` (String) When the row last changed, or null while it holds what the child was linked with.

## Import

Import is supported using the following syntax, or with an `import` block
naming the same ID. See the [importing guide](../guides/importing.md).

```shell
# A row imports by the child and the account: CHILD?account=ACCOUNT, where the child
# is its id or its name, written the way the block writes child_organization.
# While the organization has one connected account, the child alone is enough.
terraform import sreagent_github_child_access.acme_labs_acme acme-labs?account=acme
```
