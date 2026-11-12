package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// EffectiveRepoSettings is data.sreagent_effective_repo_settings: what every
// repository runs with and where each value comes from. It is a view, not a
// stored row, so it has no resource; the data source reads every page.
var EffectiveRepoSettings = engine.Spec{
	Key: "effective_repo_settings", TypeName: "effective_repo_setting", ListName: "effective_repo_settings",
	Description: "What every repository of the organization actually runs with, and where each value comes from: for each repository the GitHub App reaches, or that has settings saved, the six settings (fix_runner, platform_fallback, auto_dispatch, draft_prs, check_network, allow_ci_config) as a value plus whether it follows the organization's repository defaults. Every page is read.",
	Shape:       engine.Generated, ListIDAttr: "repo",
	Cursor: "next_cursor", PageSize: 500, MaxPages: 200,
	Attrs: []engine.Attr{
		ro("repo", engine.String, "owner/name."),
		ro("archived", engine.Bool, "Whether GitHub reports the repository archived. Archived repositories are left out unless include_archived is true."),
		ro("reachable", engine.Bool, "Whether the organization's GitHub App still reaches the repository."),
		ro("has_row", engine.Bool, "Whether the repository has settings saved (sreagent_repo_setting)."),
		ro("fence", engine.String, "The write fence: none, detected or person."),
		ro("settings", engine.JSON, "An object with one key per setting (fix_runner, platform_fallback, auto_dispatch, draft_prs, check_network, allow_ci_config), each holding value (what the repository runs with) and following (true when that is the organization default, false when the repository's own settings set it)."),
		ro("differs_by_branch", engine.StringList, "The settings a branch of the repository sets to something other than the repository's own value."),
	},
	QueryAttrs: []engine.Attr{
		{Name: "name", Kind: engine.String, Description: "A prefix of owner/name or of the repository name, or a pattern with * (infra-* matches acme/infra-a and not acme/my-infra). Not case sensitive."},
		{Name: "runner", Kind: engine.String, Description: "Only repositories whose effective fix_runner is this."},
		{Name: "has_fence", Kind: engine.Bool, Description: "true for repositories with a write fence, false for those with none."},
		{Name: "include_archived", Kind: engine.Bool, Description: "true also lists archived repositories. Default false."},
	},
	ExtraAttrs: []engine.Attr{
		{Name: "defaults", Kind: engine.JSON, Computed: true, Description: "The organization's repository defaults: values (the six settings), source (own, parent or built_in) and inherited_from."},
		{Name: "counts", Kind: engine.JSON, Computed: true, Description: "Totals over the whole filtered list: total, unreachable, without_row, with_fence and archived_hidden."},
		{Name: "runner_registered", Kind: engine.Bool, Computed: true, Description: "Whether the organization has a fix runner registered."},
		{Name: "archived_known", Kind: engine.Bool, Computed: true, Description: "False when GitHub's archived flags could not be read fresh, so the archived filter may be stale."},
	},
}
