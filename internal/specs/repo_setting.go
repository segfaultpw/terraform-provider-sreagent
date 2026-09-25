package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

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
		{Name: "path_prefix", Kind: engine.String, Description: "Only paths under this prefix may be changed."},
		{Name: "writable_extensions", Kind: engine.StringList, Description: "File extensions a fix may write."},
		{Name: "alerting_sns_topic_arn", Kind: engine.String, Description: "An SNS topic alarms for this repository publish to."},
		{Name: "fix_runner", Kind: engine.String, Description: "platform, opencode_ci or self_hosted."},
	},
	ListIDAttr: "repo",
	ListAttrs: []engine.Attr{
		{Name: "repo", Kind: engine.String, Computed: true, Description: "owner/name."},
		{Name: "fix_runner", Kind: engine.String, Computed: true, Description: "The repository's fix runner."},
		{Name: "branches", Kind: engine.JSON, Computed: true, Description: "The repository's branch mappings."},
	},
}
