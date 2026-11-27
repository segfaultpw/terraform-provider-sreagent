package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// An organization can connect more than one GitHub account, and a parent
// organization decides, per child and per account, which repositories the child
// may use. Neither row can be created or deleted by an API key: an account is
// connected on GitHub and in the app, and a child is linked under its parent by a
// platform administrator. So both resources adopt a row that exists and write only
// what the block states, and a destroy leaves the row in place.

// GitHubAccount mirrors x-sreagent-resources.github_accounts: the settings of one
// connected GitHub account (one installation). sreagent_github_settings manages
// the same row for the organization's managed account, the one connected longest.
var GitHubAccount = engine.Spec{
	Key: "github_accounts", TypeName: "github_account", ListName: "github_accounts",
	Description: "The settings of one GitHub account the organization has connected: open fix pull requests automatically, revise a pull request when a review asks, open them as drafts, review pull requests, comment links on linked issues, \"Auto-merge only what SRE Agent verified\" and the lowest severity a review comments on. It is the per-account form of sreagent_github_settings.\n\n" +
		"Which one to use: sreagent_github_settings manages the organization's managed account, the one connected longest, and the organization's context repository. Use sreagent_github_account for any other account of an organization that connected more than one. A block for the managed account fights sreagent_github_settings (both write the same row), so the plan warns when the account it adopts is the managed one, and the platform's exported configuration writes this resource only for the accounts that are not the managed one. The managed account is not fixed: it is the oldest account the organization has connected, so when that account is disconnected the next oldest becomes the managed one. A block for that next account, which did not overlap sreagent_github_settings before, now does (the plan warns on it from the next refresh): then manage that account with sreagent_github_settings and remove the block. An organization with one account manages it with sreagent_github_settings.\n\n" +
		"The platform has no door that connects or disconnects an account, so the resource adopts one that is connected: creating it reads the account by its login and writes only the settings the block states and the account does not already hold, and a setting the block leaves out is left as it is. An account that is not connected is an error that says where to connect it (the Integrations page). Destroying the resource leaves the account connected and its settings in place; disconnect it on the Integrations page. The context repository is not an attribute here: it belongs to the organization, and sreagent_github_settings sets it.\n\n" +
		"Import it with the account's login as the platform shows it (`terraform import sreagent_github_account.NAME acme-labs`). A GitHub account that is renamed on GitHub changes its login, so after a rename the block is edited to the new login and imported again. An account the organization reads through its parent (a child organization) is not its own row: its settings are the parent organization's, and this resource cannot adopt it. Needs the GitHub integration in the plan and an api:admin key; a plan-only api:config_read key plans and cannot apply.",
	Shape: engine.NaturalKey, IDAttr: "account", Lifecycle: true, MinPlatform: "v0.538.0",
	ReadDescription: "Every GitHub account the organization has connected, with its settings (open fix pull requests automatically, revise a pull request when a review asks, open them as drafts, review pull requests, comment links on linked issues, \"Auto-merge only what SRE Agent verified\" and the lowest severity a review comments on), whether it is the managed account (the one sreagent_github_settings manages, the one connected longest) and the repositories it reaches (at most 500 each). A child organization lists only its own accounts, never its parent's. This data source only reads; sreagent_github_account manages the settings of an account.",
	AdoptOnly:       true, AdoptHint: "Connect the account on the Integrations page first.",
	LeaveOnDestroy: "The account stays connected and keeps these settings; change them here or in the app, and disconnect the account on the Integrations page.",
	PlanChecks:     []func(plan, prior engine.Config) []engine.Issue{managedAccountWarning},
	Attrs: []engine.Attr{
		{Name: "account", Kind: engine.String, Address: true, Required: true, CreateOnly: true, Description: "The account's login on GitHub, as the Integrations page shows it (the platform ignores case). Changing it replaces the resource: the old account keeps its settings and the block adopts the other one."},
		{Name: "auto_pr_enabled", Kind: engine.Bool, UpdateOnly: true, Description: "Open fix pull requests automatically."},
		{Name: "auto_revise_on_review", Kind: engine.Bool, UpdateOnly: true, Description: "Revise a pull request when a review asks."},
		{Name: "draft_prs", Kind: engine.Bool, UpdateOnly: true, Description: "Open fix pull requests as drafts. It is this account's choice; a repository's own draft_prs (sreagent_repo_setting) and the organization default (sreagent_repository_defaults) are read first. Once set, Terraform cannot return it to null; do that in the app."},
		{Name: "review_enabled", Kind: engine.Bool, UpdateOnly: true, Description: "Review pull requests."},
		{Name: "link_comments_enabled", Kind: engine.Bool, UpdateOnly: true, Description: "Comment links on linked issues."},
		{Name: "auto_merge_requires_verification", Kind: engine.Bool, UpdateOnly: true, Description: "Auto-merge only what SRE Agent verified. Off by default: a fix SRE Agent's own checks did not run for can still merge by itself once the checks its branch requires pass (where a repository's auto_merge says so), and a change pushed onto a pull request afterwards merges on those checks alone. An organization admin takes that risk by leaving it off. On: only a fix whose exact commit those checks ran on and passed merges by itself. In both positions a fix whose checks failed, or ran and could not judge it, or has an open self-review blocker, never merges by itself."},
		{Name: "review_severity_floor", Kind: engine.String, UpdateOnly: true, OneOf: []string{"critical", "high", "medium", "low"}, Description: "The lowest severity a review comments on."},
		{Name: "account_id", Kind: engine.Int, Computed: true, Description: "GitHub's id of the account, or null when it has not been recorded yet."},
		{Name: "account_type", Kind: engine.String, Computed: true, Description: "User or Organization, as GitHub reports it, or null when it has not been recorded yet."},
		{Name: "installation_id", Kind: engine.Int, Computed: true, Description: "The GitHub App installation id of the account."},
		{Name: "status", Kind: engine.String, Computed: true, Description: "active, or suspended while GitHub has the installation suspended."},
		{Name: "managed", Kind: engine.Bool, Computed: true, Description: "True for the organization's managed account, the one sreagent_github_settings manages."},
		{Name: "holds_context_repo", Kind: engine.Bool, Computed: true, Description: "True when the organization's context repository is read through this account."},
		{Name: "repository_count", Kind: engine.Int, Computed: true, Description: "How many repositories the account reaches, as of the last time the platform read the list."},
		{Name: "repos_truncated", Kind: engine.Bool, Computed: true, Description: "True when repos holds fewer repositories than the account reaches (the platform answers at most 500)."},
		{Name: "repos_refreshed_at", Kind: engine.String, Computed: true, Description: "When the platform last read the account's repository list, or null when it never has."},
		{Name: "repos", Kind: engine.JSON, Computed: true, Description: "The repositories the account reaches, at most 500, as the platform names them: a JSON array, decoded with jsondecode."},
	},
}

// managedAccountWarning says what a block for the managed account does: it writes
// the row sreagent_github_settings writes, so the two plan against each other.
func managedAccountWarning(_, prior engine.Config) []engine.Issue {
	if prior == nil {
		return nil
	}
	managed, presence := prior.Bool("managed")
	if presence != engine.Present || !managed {
		return nil
	}
	return []engine.Issue{{Attr: "account", Warning: true, Summary: "This is the organization's managed account",
		Detail: "The account's settings are also managed by sreagent_github_settings, the resource for the organization's managed account (the one connected longest). A block for each writes the same row, so they plan against each other whenever they state different values. Manage the managed account with sreagent_github_settings and use sreagent_github_account only for the organization's other accounts."}}
}

// GitHubChildAccess mirrors x-sreagent-resources.github_child_access: what a
// parent organization gives one child organization of one of its GitHub accounts.
// A change that takes repositories away from the child is written against a
// preview, the way repository_defaults is, and one that takes nothing away is
// written at once.
var GitHubChildAccess = engine.Spec{
	Key: "github_child_access", TypeName: "github_child_access", ListName: "github_child_access",
	Description: "What the parent organization gives one child organization of one of the parent's GitHub accounts: all of the account's repositories (mode all), only the ones named (selected, at most 500) or none, and whether the child may use the parent's context repository. One block for each child and account.\n\n" +
		"Use it in the parent organization. A child reads only what it is given here: a child linked under the parent is given all of the accounts connected at that moment, except that a child organization that has its own GitHub account is given nothing of its parent's accounts until an admin of the parent gives it repositories here. An account connected later reaches no existing child until it is given. A repository the child is not given is never read, minted a token for or opened a pull request on for the child.\n\n" +
		"The platform has no door that creates or deletes the row (a child is linked under its parent by a platform administrator, and the account is connected in the app), so the resource adopts it: creating it reads the row, judges the block against it and writes only the difference, and a setting the block leaves out is left as it is. A child or account that does not exist is an error that says where to link or connect it. Destroying the resource leaves the row in place and changes nothing; the child keeps what it was given, so to take access away state mode = \"none\" (or narrow it) and apply.\n\n" +
		"A change that takes repositories away from the child is applied only against a preview of what it moves, so terraform plan asks the platform for the preview (it needs an api:admin key; with an api:config_read key the plan says so, and the apply previews and applies) and warns with what the child may use now and will use, the repositories it stops reaching, and what is held on them: open fix pull requests stay open on GitHub with one comment saying SRE Agent no longer manages them, auto-merge is turned off, and waiting requests and the cards are named. Nothing is closed or deleted. The apply sends the plan's fingerprint and fails without writing when what would move changed since the plan; plan again. Where the plan could not take the preview (an api:config_read key, or a child or account known only at apply) the apply takes it and applies what it finds, with a warning, so review the plan with an api:admin key first. A change that takes nothing away (giving the child more, or changing only context_repo_shared) is applied at once.\n\n" +
		"Import it as `CHILD_ID?account=ACCOUNT` (`terraform import sreagent_github_child_access.NAME CHILD_ID?account=acme`); without the account an organization with one connected account takes that one. CHILD_ID is the child's id or its name: write it the way the block writes child_organization, and the account the way the block writes account. A block that names the same row another way (the child's name for its id, another case of the login) is not a new row: the plan shows one in-place update that sends nothing and never a replacement. The key's organization must be the parent. Needs the GitHub integration in the plan.",
	Shape: engine.PathAndQuery, IDAttr: "child_organization", QueryAttr: "account", Lifecycle: true, MinPlatform: "v0.538.0",
	AdoptOnly: true, AdoptHint: "Link the child organization under this organization (a platform administrator does this), and connect the GitHub account on the Integrations page.",
	Previewed: true, ProbeFingerprint: "preview", AccessPreview: true,
	ReadDescription: "What the parent organization gives each child organization of each of its GitHub accounts: one row for each child and account, with the mode (all, selected or none), the repositories of a selected child, whether the child may use the parent's context repository, how many repositories it reaches now and whether the grant applies (reason says why not). Read it in the parent organization. This data source only reads; sreagent_github_child_access manages a row.",
	LeaveOnDestroy:  "The child keeps what it was given of the account; change it here or in the app.",
	Checks:          []func(engine.Config) []engine.Issue{repositoriesOnlyWithSelected},
	Attrs: []engine.Attr{
		{Name: "child_organization", Kind: engine.String, Address: true, NamedBy: []string{"child_name"}, Required: true, CreateOnly: true, Description: "The child organization, by its id or its name. Changing it replaces the resource: the old row keeps what it holds."},
		{Name: "account", Kind: engine.String, Address: true, NamedBy: []string{"installation_id"}, Required: true, CreateOnly: true, Description: "The parent's GitHub account, by its login as the Integrations page shows it (the platform ignores case) or its installation number. Changing it replaces the resource."},
		{Name: "mode", Kind: engine.String, UpdateOnly: true, OneOf: []string{"all", "selected", "none"}, Description: "What the child may use of the account: all (every repository the account reaches, now and later), selected (only the repositories in repositories) or none. Choosing all or none while repositories lists some is an error."},
		{Name: "repositories", Kind: engine.StringList, UpdateOnly: true, Unordered: true, Normalize: engine.Lower, Recomputed: true, MaxItems: 500, Description: "The repositories a selected child may use, at most 500, as owner/name: the plan refuses a repository the account does not reach, with the platform's sentence. Only with mode = \"selected\" (the platform answers an empty list for all and none, and the attribute follows the mode). The order and the case do not matter (acme/web and Acme/Web are one repository, and naming both is refused); state keeps the spelling written here, and repository_details shows the platform's. A repository the account connects later is not given until it is named here."},
		{Name: "context_repo_shared", Kind: engine.Bool, UpdateOnly: true, Description: "Whether the child may use the parent's context repository (the one sreagent_github_settings sets), through an account it was given something of. It is the child's setting, not the account's: the platform holds one value for each child whichever account's row states it. State it on one block of the child only, because two blocks of one child that state different values change it on every apply. On by default."},
		{Name: "child_name", Kind: engine.String, Computed: true, Description: "The child organization's name."},
		{Name: "installation_id", Kind: engine.Int, Computed: true, Description: "The GitHub App installation id of the parent's account."},
		{Name: "repository_count", Kind: engine.Int, Computed: true, Description: "How many repositories of the account the child reaches now."},
		{Name: "details_read", Kind: engine.Bool, Computed: true, Description: "True when the platform has read the account's repository list for a selected child, so repository_details is complete."},
		{Name: "applies", Kind: engine.Bool, Computed: true, Description: "True when the child reads this grant. False when it cannot yet (reason says why)."},
		{Name: "reason", Kind: engine.String, Computed: true, Description: "Why the grant does not apply, or null while it does: for example the child stopped using the parent's GitHub, or it connected its own GitHub account and was given nothing at the time."},
		{Name: "source", Kind: engine.String, Computed: true, Description: "How the row came to be: for example link (made when the child was linked), shipped, own_at_cutoff (the child already had a GitHub account of its own), or a change by a person or a key."},
		{Name: "stopped", Kind: engine.Bool, Computed: true, Description: "True when the child chose to stop using its parent's GitHub, so none of this applies until it resumes."},
		{Name: "own_github", Kind: engine.Bool, Computed: true, Description: "True when the child holds a GitHub account of its own. The parent never learns which."},
		{Name: "updated_at", Kind: engine.String, Computed: true, Description: "When the row last changed, or null while it holds what the child was linked with."},
		{Name: "repository_details", Kind: engine.JSON, Computed: true, Description: "For a selected child, each repository it was given with whether the account still reaches it, as the platform answers it, as a JSON array decoded with jsondecode."},
		{Name: "preview_fingerprint", Kind: engine.String, Computed: true, Local: true, Description: "The fingerprint of the preview the plan took, which the apply sends. Kept by the provider; never configure it."},
	},
}

// repositoriesOnlyWithSelected is the platform's rule for a list it answers as
// empty for every mode but selected.
func repositoriesOnlyWithSelected(c engine.Config) []engine.Issue {
	mode, mp := c.String("mode")
	repos, rp := c.Strings("repositories")
	if mp != engine.Present || rp != engine.Present {
		return nil
	}
	switch {
	case mode != "selected" && len(repos) > 0:
		return []engine.Issue{{Attr: "repositories", Summary: "Repositories need mode selected",
			Detail: "The platform takes a list of repositories only with mode = \"selected\" (it answers an empty list for all and none). Set mode = \"selected\", or remove repositories."}}
	case mode == "selected" && len(repos) == 0:
		return []engine.Issue{{Attr: "repositories", Summary: "Name the repositories to give",
			Detail: "mode = \"selected\" with no repositories gives the child nothing. Name the repositories, or choose mode = \"all\" or \"none\"."}}
	}
	return nil
}
