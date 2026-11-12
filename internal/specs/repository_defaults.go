package specs

import "github.com/segfaultpw/terraform-provider-sreagent/internal/engine"

// RepositoryDefaults mirrors x-sreagent-resources.repository_defaults: the six
// settings every repository without its own value follows. The platform applies
// a write only against a preview, so the resource previews at plan and applies
// the fingerprint of that preview.
var RepositoryDefaults = engine.Spec{
	Key: "repository_defaults", TypeName: "repository_defaults", ListName: "repository_defaults",
	Description: "The organization's repository defaults: the six settings (fix_runner, platform_fallback, auto_dispatch, draft_prs, check_network, allow_ci_config) every repository follows until it holds a value of its own (sreagent_repo_setting).\n\n" +
		"The platform applies a change only against a preview of what it moves, so terraform plan asks for that preview (it needs an api:admin key; with an api:config_read key the plan says so and the apply previews and applies) and warns with the settings that change, the repositories that read a different value and the child organizations that follow these defaults. The apply sends the plan's fingerprint, and fails without writing when what would move changed since. Turning allow_ci_config or check_network on needs the matching entry in confirm.\n\n" +
		"One row per organization: creating the resource adopts the current defaults, a setting the configuration leaves out is left as it is, and draft_prs cannot be put back to the installation's setting from Terraform (do it in the app). An organization with a parent that follows the parent's defaults has no row of its own: creating the resource gives it its own for all six settings and stops it following the parent, and destroying the resource follows the parent again. In an organization with no parent, destroying the resource leaves the defaults in place.",
	Shape: engine.Singleton, Lifecycle: true, Previewed: true, MinPlatform: "v0.477.0",
	Confirmations: map[string]engine.Confirmation{
		"ci_config":     {Attr: "allow_ci_config", Sentence: "Turning on fix changes to CI configuration for every repository that follows the default needs confirmation: pass confirm ci_config."},
		"check_network": {Attr: "check_network", Sentence: "Turning on network access for checks in every repository that follows the default needs confirmation: pass confirm check_network."},
	},
	DestroyBody:    map[string]any{"follow_parent": true},
	DestroyWhen:    "parent_name",
	DestroyWarning: "This organization follows its parent's repository defaults again; the values it had of its own are dropped.",
	Attrs: []engine.Attr{
		{Name: "fix_runner", Kind: engine.String, Description: "Who executes the fix requests of repositories that follow: platform (SRE Agent's own agent) or self_hosted (a runner on your own machine, which needs one registered for the organization; included in the Business and Enterprise plans)."},
		{Name: "platform_fallback", Kind: engine.Bool, Description: "true lets a request a following repository's own runner could not finish run on SRE Agent's own runner. false keeps the code on your machines: the request fails with a plain sentence instead."},
		{Name: "auto_dispatch", Kind: engine.Bool, Description: "true sends a following repository's fix requests as soon as they are made. false holds each until a person presses Send."},
		{Name: "draft_prs", Kind: engine.Bool, Description: "Whether a following repository's fix pull request opens as a draft (true) or ready for review (false). Null means the GitHub installation's setting decides; once set, Terraform cannot return it to null, do that in the app."},
		{Name: "check_network", Kind: engine.Bool, Description: "Whether the commands after setup (checks, tests, analysis) of following repositories may reach GitHub and the package registries while a fix is checked. Turning it on needs confirm to name check_network and is accepted only where the Integrations page offers it."},
		{Name: "allow_ci_config", Kind: engine.Bool, Description: "Whether fixes may change CI configuration inside the write fence of following repositories. Turning it on needs confirm to name ci_config. With this on, a fix may change CI configuration, and the repository's CI runs that change as soon as the branch is pushed, with the repository's secrets, before anyone reviews the pull request."},
		{Name: "confirm", Kind: engine.StringList, WriteOnly: true, OneOf: []string{"ci_config", "check_network"}, Description: "The confirmations a change needs: ci_config when it turns allow_ci_config on, check_network when it turns check_network on. The plan fails without the one the preview asks for. Naming a confirmation never writes anything by itself."},
		{Name: "source", Kind: engine.String, Computed: true, Description: "Where the values come from: own (this organization set them), parent (a child organization follows its parent's) or built_in (none set)."},
		{Name: "parent_name", Kind: engine.String, Computed: true, Description: "The parent organization's name, or null for an organization with none."},
		{Name: "stopped", Kind: engine.Bool, Computed: true, Description: "True when a child organization chose to stop following its parent."},
		{Name: "updated_at", Kind: engine.String, Computed: true, Description: "When the organization's own defaults last changed, or null."},
		{Name: "preview_fingerprint", Kind: engine.String, Computed: true, Local: true, Description: "The fingerprint of the preview the plan took, which the apply sends. Kept by the provider; never configure it."},
	},
}
