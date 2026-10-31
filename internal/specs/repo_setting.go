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

// RepoSetting mirrors x-sreagent-resources.repo_settings: one row per branch
// mapping. The list groups mappings by repository, so the list data source
// reads repo, fix_runner and branches.
var RepoSetting = engine.Spec{
	Key: "repo_settings", TypeName: "repo_setting", ListName: "repo_settings",
	Description: "One repository branch mapping for fix requests. fix_runner is per repository: every mapping of one repository must name the same runner. Creating one needs the organization's GitHub App to reach the repository.",
	Shape:       engine.Generated, Lifecycle: true,
	Attrs: []engine.Attr{
		{Name: "repo", Kind: engine.String, Required: true, CreateOnly: true, Description: "owner/name; must be reachable by the organization's GitHub App."},
		{Name: "branch", Kind: engine.String, Description: "The branch; omit for the all-branches row."},
		{Name: "service", Kind: engine.String, Description: "The service this repository deploys."},
		{Name: "environment", Kind: engine.String, Description: "The environment this branch deploys to."},
		{Name: "path_prefix", Kind: engine.String, Recomputed: true, Description: "One directory a proposed fix may change. Sending it replaces the whole list with that one directory. Answered when the fence has exactly one directory and null otherwise; use path_prefixes for several, and set only one of the two."},
		{Name: "path_prefixes", Kind: engine.StringList, Recomputed: true, MaxItems: 20, Description: "The directories a proposed fix may change, relative to the repository root, such as [\"services/api/\", \"services/web/\"], or [\".\"] for the whole repository. At most 20; no entry may sit inside another, start with a slash or contain \"..\". Sending it replaces the whole list; [] clears it, and the directories are then detected from the repository's layout the first time a fix runs. Set only one of path_prefix and path_prefixes."},
		{Name: "allow_ci_config", Kind: engine.Bool, Description: "Whether fixes may change CI configuration (.github/, .circleci/, .gitlab-ci.yml, Jenkinsfile and similar) inside the directories. Off by default. Each such change opens as a draft pull request that names the CI files and is never merged by SRE Agent; with this on, the repository's CI runs that change, with the repository's secrets, as soon as the branch is pushed and before anyone reviews the pull request."},
		{Name: "writable_extensions", Kind: engine.StringList, Description: "File extensions a fix may write."},
		{Name: "alerting_sns_topic_arn", Kind: engine.String, Description: "An SNS topic alarms for this repository publish to."},
		{Name: "fix_runner", Kind: engine.String, Description: "Who executes the repository's fix requests: platform (SRE Agent's own agent) or self_hosted (a runner on your own machine, which needs one registered for the organization). Applies to the whole repository, so every mapping of it must name the same runner."},
		{Name: "auto_dispatch", Kind: engine.Bool, Description: "Whether fix requests for this repository are sent to the runner automatically; false holds each one until a person sends it. Defaults to true."},
		{Name: "ci_fallback_to_platform", Kind: engine.Bool, Description: "Stored and unused: the platform no longer reads it. Defaults to false."},
		{Name: "platform_fallback", Kind: engine.Bool, Description: "Defaults to true: a request this repository's own runner could not finish may run on SRE Agent's own runner, so it still gets an answer. false keeps the repository's code on your machines and the request fails instead. Applies to the whole repository."},
		{Name: "draft_prs", Kind: engine.Bool, Clearable: true, Description: "Whether a fix pull request for this row opens as a draft (true), ready for review (false), or as the GitHub installation's draft_prs setting says (unset, the default). A branch row that leaves it unset follows the repository's all-branches row. A change the sandbox saw fail, an open self-review blocker or CI configuration files still open as a draft whatever this says."},
		{Name: "check_network", Kind: engine.Bool, Description: "Whether the repository's commands after setup (checks, tests, analysis) may reach GitHub and the package registries while a fix is checked or verified. Off by default. On, those commands, and code a fix writes and runs, can send the repository's source or command output to any account on those hosts. Turning it on is accepted only where the Integrations page offers it; turning it off always is."},
	},
	Checks:     []func(engine.Config) []engine.Issue{fenceChecks},
	ListIDAttr: "repo",
	ListAttrs: []engine.Attr{
		{Name: "repo", Kind: engine.String, Computed: true, Description: "owner/name."},
		{Name: "fix_runner", Kind: engine.String, Computed: true, Description: "The repository's fix runner."},
		{Name: "branches", Kind: engine.JSON, Computed: true, Description: "The repository's branch mappings."},
	},
}
