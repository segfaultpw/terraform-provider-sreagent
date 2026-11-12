package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// fenceChecks mirrors the platform's refusal of a write that names both ways
// of stating the write fence: path_prefix is one directory and path_prefixes
// is the whole list.
func fenceChecks(c engine.Config) []engine.Issue {
	_, one := c.String("path_prefix")
	if one != engine.Present || c.List("path_prefixes") != engine.Present {
		return nil
	}
	return []engine.Issue{{Attr: "path_prefix", Summary: "Not allowed with path_prefixes",
		Detail: "path_prefix is one directory and path_prefixes is the whole list; the platform refuses a write that sends both. Keep path_prefixes and remove path_prefix."}}
}

// repoSettingNames is the six settings that can follow the organization
// default, in the order the platform lists them.
var repoSettingNames = []string{"fix_runner", "platform_fallback", "auto_dispatch", "draft_prs", "check_network", "allow_ci_config"}

// RepoSetting mirrors x-sreagent-resources.repo_settings: one row per branch
// mapping. The list groups mappings by repository, so the list data source
// reads repo, fix_runner and branches.
var RepoSetting = engine.Spec{
	Key: "repo_settings", TypeName: "repo_setting", ListName: "repo_settings",
	Description: "One repository branch mapping for fix requests. Creating one needs the organization's GitHub App to reach the repository.\n\n" +
		"fix_runner, platform_fallback, auto_dispatch, draft_prs, check_network and allow_ci_config each either belong to this row or follow the organization's repository defaults (sreagent_repository_defaults). set_here lists the ones the row holds; following lists the rest.\n\n" +
		"For fix_runner, platform_fallback, auto_dispatch, check_network and allow_ci_config, a setting the configuration leaves out is left as it is, whether the row holds a value or follows, and a setting it states with the value the row already reads is not written, so a file that restates what a row follows does not pin it. A stated value is kept, though: when the default later changes, the next apply writes the stated value and the row then holds it, so remove a stated value that should follow the default. To put a setting back on the default, remove it from the configuration and name it in follows_default.\n\n" +
		"draft_prs is the exception: leaving it out returns it to the default (a row that holds a value plans a change to null), and stating it writes it, even with the value the row follows, so the row then holds it.\n\n" +
		"fix_runner, platform_fallback and auto_dispatch belong to the whole repository: a write or a reset of one reaches every row of the repository, branch entries included (the plan warns), and every mapping of one repository must name the same runner. State them on the repository's all-branches block only, and leave them out of its other blocks: a block that leaves one out follows along with no change, a block that states a different value fights the other, and two blocks that both change one in a single apply need a second apply, because the first write changes the version of the other row and the second write is refused (412).",
	Shape: engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "repo", Kind: engine.String, Required: true, CreateOnly: true, Description: "owner/name; must be reachable by the organization's GitHub App."},
		{Name: "branch", Kind: engine.String, Description: "The branch; omit for the all-branches row."},
		{Name: "service", Kind: engine.String, Description: "The service this repository deploys."},
		{Name: "environment", Kind: engine.String, Description: "The environment this branch deploys to."},
		{Name: "path_prefix", Kind: engine.String, Recomputed: true, Description: "One directory a proposed fix may change. Sending it replaces the whole list with that one directory. Answered when the fence has exactly one directory and null otherwise; use path_prefixes for several, and set only one of the two."},
		{Name: "path_prefixes", Kind: engine.StringList, Recomputed: true, MaxItems: 20, Description: "The directories a proposed fix may change, relative to the repository root, such as [\"services/api/\", \"services/web/\"], or [\".\"] for the whole repository. At most 20; no entry may sit inside another, start with a slash or contain \"..\". Sending it replaces the whole list; [] clears it, and the directories are then detected from the repository's layout the first time a fix runs. Set only one of path_prefix and path_prefixes."},
		{Name: "allow_ci_config", Kind: engine.Bool, Description: "Whether fixes may change CI configuration (.github/, .circleci/, .gitlab-ci.yml, Jenkinsfile and similar) inside the directories. Off unless the organization default says otherwise. Each such change opens as a draft pull request that names the CI files and is never merged by SRE Agent; with this on, the repository's CI runs that change, with the repository's secrets, as soon as the branch is pushed and before anyone reviews the pull request."},
		{Name: "writable_extensions", Kind: engine.StringList, Description: "File extensions a fix may write."},
		{Name: "alerting_sns_topic_arn", Kind: engine.String, Description: "An SNS topic alarms for this repository publish to."},
		{Name: "fix_runner", Kind: engine.String, Description: "Who executes the repository's fix requests: platform (SRE Agent's own agent) or self_hosted (a runner on your own machine, which needs one registered for the organization). Applies to the whole repository, so every mapping of it must name the same runner."},
		{Name: "auto_dispatch", Kind: engine.Bool, Description: "Whether fix requests for this repository are sent to the runner automatically; false holds each one until a person sends it. Follows the organization default (true unless it was changed) while the row does not hold a value."},
		{Name: "ci_fallback_to_platform", Kind: engine.Bool, Description: "Stored and unused: the platform no longer reads it. Defaults to false."},
		{Name: "platform_fallback", Kind: engine.Bool, Description: "Follows the organization default (true unless it was changed) while the row does not hold a value: a request this repository's own runner could not finish may run on SRE Agent's own runner, so it still gets an answer. false keeps the repository's code on your machines and the request fails instead. Applies to the whole repository."},
		{Name: "draft_prs", Kind: engine.Bool, Clearable: true, NullUnlessIn: "set_here", Description: "Whether a fix pull request for this row opens as a draft (true), ready for review (false), or as the organization's repository defaults and the GitHub installation's draft_prs setting say (unset, the default). Reads null while the row follows the default, so a configuration that leaves it out plans nothing; read data.sreagent_effective_repo_settings for the value in force. Unlike the other settings, stating it writes it even with the value the row follows, and removing it from a row that holds a value sends null, which puts it back on the default; naming it in follows_default means the same. A branch row that leaves it unset follows the repository's all-branches row. A change the sandbox saw fail, an open self-review blocker or CI configuration files still open as a draft whatever this says."},
		{Name: "check_network", Kind: engine.Bool, Description: "Whether the repository's commands after setup (checks, tests, analysis) may reach GitHub and the package registries while a fix is checked or verified. Off unless the organization default says otherwise. On, those commands, and code a fix writes and runs, can send the repository's source or command output to any account on those hosts. Turning it on is accepted only where the Integrations page offers it; turning it off always is."},
		{Name: "follows_default", Kind: engine.StringList, WriteOnly: true, OneOf: repoSettingNames, Description: "Settings to put back on the organization's repository defaults, from among the six above. Applied only to a setting the row holds itself (set_here); one that already follows changes nothing, so the list never plans a change by itself and an imported row plans nothing for it. A setting named here must not also be stated. On a platform before v0.482.0 the list is ignored and the plan says so. The platform refuses a reset to a value it would refuse to set: your own runner with none registered, network where the Integrations page does not offer it, and CI configuration off while the directories list a CI path. fix_runner, platform_fallback and auto_dispatch are reset on every row of the repository."},
		{Name: "set_here", Kind: engine.StringList, Computed: true, Description: "The settings this row holds itself, from among fix_runner, platform_fallback, auto_dispatch, draft_prs, check_network and allow_ci_config. Null on a platform from before release v0.482.0."},
		{Name: "set_here", TFName: "following", Kind: engine.StringList, Computed: true, Complement: repoSettingNames, Description: "The settings this row does not hold: it reads the organization's repository defaults for them. Null on a platform from before release v0.482.0."},
	},
	Checks:             []func(engine.Config) []engine.Issue{fenceChecks},
	FollowsDefault:     "follows_default",
	SetHere:            "set_here",
	FollowsMinPlatform: "v0.482.0",
	RepositoryWide:     []string{"fix_runner", "platform_fallback", "auto_dispatch"},
	RepositoryAttr:     "repo",
	ListIDAttr:         "repo",
	ListAttrs: []engine.Attr{
		{Name: "repo", Kind: engine.String, Computed: true, Description: "owner/name."},
		{Name: "fix_runner", Kind: engine.String, Computed: true, Description: "The repository's fix runner."},
		{Name: "branches", Kind: engine.JSON, Computed: true, Description: "The repository's branch mappings."},
	},
}
