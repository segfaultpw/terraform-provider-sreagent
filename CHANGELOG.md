## [0.2.0]

FEATURES:

* `sreagent_repository_defaults` (and `data.sreagent_repository_defaults`) manages the organization's repository defaults: the six settings (`fix_runner`, `platform_fallback`, `auto_dispatch`, `draft_prs`, `check_network`, `allow_ci_config`) every repository follows until it holds a value of its own. The platform applies a change only against a preview of what it moves, so `terraform plan` asks for the preview and warns with the settings that change, the repositories that would read a different value and the child organizations that follow the defaults, and the apply sends the fingerprint of that preview and fails without writing when what would move changed since. Turning `allow_ci_config` or `check_network` on needs the matching entry in the write-only `confirm` list, and the plan fails without it. With an `api:config_read` plan key the plan says it cannot preview and the apply previews and applies what it finds. A child organization that follows its parent's defaults has no row of its own: creating the resource gives it its own for all six settings and stops it following the parent (the plan says so), importing it fails and says why, and destroying the resource follows the parent again; in an organization with no parent, destroying it leaves the defaults in place. Needs platform v0.477.0.
* `sreagent_repo_setting` takes the write-only `follows_default` (put the named settings back on the organization's repository defaults) and answers `set_here` (the settings the row holds) and `following` (the ones it does not). A setting the configuration leaves out is left as it is, and one it states with the value the row already reads is not written, so a configuration written before the defaults existed does not pin what the rows follow. `fix_runner`, `platform_fallback` and `auto_dispatch` belong to the whole repository: a reset of one reaches every row of the repository. Needs platform v0.482.0; an older platform answers neither list and is never sent `follows_default`.
* `data.sreagent_effective_repo_settings` reads what every repository runs with and where each value comes from, every page, with the filters `name`, `runner`, `has_fence` and `include_archived`, and the organization's `defaults`, the `counts` of the whole filtered list, `runner_registered` and `archived_known`.
* Data sources for the platform's security and code detection settings: `sreagent_code_detection_settings`, `sreagent_security_sla_policy`, `sreagent_security_digest_settings`, `sreagent_security_code_classes`, `sreagent_security_notification_rules` and `sreagent_secret_allow_rules`. They are read only; managing them from Terraform is planned for a later release. An older platform fails the read and names the release it needs.
* `sreagent_organization_settings` answers `mfa_required`, `mfa_grace_days`, `session_idle_minutes`, `api_key_max_days`, `mfa_effective` and `session_idle_effective` (all read only: an org admin decides them in the app), `sreagent_slack` answers `delivery_problems`, and `data.sreagent_compliance_periods` answers `state`, `frozen_at`, `freeze_kind`, `catalogue_version` and `fill_present_with_current`.
* The vendored API document is re-vendored whole, which ends the daily contract drift failure that named the resources above as missing from the provider.

BUG FIXES:

* `sreagent_repo_setting` no longer plans a change on every run when the row follows an organization default that sets `draft_prs`: a following `draft_prs` reads null when the configuration does not state it.
* `sreagent_organization_settings` plans clean in a child organization that stopped following its parent's repository defaults. The platform keeps `repository_defaults` in `inheritance_stopped` through every write of the list; the provider no longer reads it, sends it or accepts it there (a configuration that names it is refused at validate; the stop is changed with `sreagent_repository_defaults` or in the app), so a configuration that states the list no longer fails the apply as an inconsistent result.

BREAKING CHANGES:

* `sreagent_repo_setting.draft_prs` on a row that follows the organization default reads null when the configuration does not state it, where 0.1.3 read the value in force. A configuration that reads `sreagent_repo_setting.x.draft_prs` to learn the value a repository runs with should read `data.sreagent_effective_repo_settings` instead; one that states `draft_prs` is unchanged.

UPGRADE NOTES:

* Upgrading plans nothing for a configuration that states values equal to what its rows follow: a stated value the row already reads is not written. To make a row follow a default again, remove the setting from the configuration and name it in `follows_default`; the plan warns that the apply puts it back.
* State a repository-wide setting (`fix_runner`, `platform_fallback`, `auto_dispatch`) the same way on every `sreagent_repo_setting` of a repository, or on the all-branches block only: a reset of one reaches every row of the repository, so a block that states a different value fights it.
* Terraform plans a change again when it applies a saved plan, so a change to `sreagent_repository_defaults` asks the platform for its preview at the plan and again at the apply. If what would move changed in between, the apply stops with Terraform's "inconsistent final plan" error or the platform's 409; either way nothing was written. Run the plan again.
* The new platform parts need the releases in the README: v0.477.0 for the repository defaults and the view of effective settings, v0.482.0 for `follows_default`, `set_here` and `following`.

## [0.1.3]

FEATURES:

* `sreagent_outbound_config` takes `card_events` (webhook targets only): also send the organization's card approval events to the target. The platform refuses it for any other `provider_type`, and it is answered on `data.sreagent_outbound_configs`. Needs a platform release that carries it; the vendored API document is updated with the field.

## [0.1.2]

FEATURES:

* `sreagent_organization_settings` and `data.sreagent_organization_settings` answer `fix_request_floor` (`org_admin` or `member`): who may ask SRE Agent for a fix pull request, or to change one it opened. It is read only: an org admin changes it in the app, the platform refuses it from an API key, so the provider reads it into state and never sends it, and a configuration that names it is refused at validate.
* `sreagent_organization_settings` takes `inheritance_stopped`, the whole set of a parent organization's connections (`slack`, `github_installation`, `ai_providers`, `outbound_targets`, `ticket_integrations`) that a child organization does not use. It is sent only when configured, and the platform lists it as writable in its API document, but it refuses a change of the set from an API key (the same rule as the two sign-up policies), so a value that differs from the stored one fails the apply with the platform's sentence; naming the set as it already is passes. Both need a platform release that carries the fields, which is newer than v0.469.2; an older platform answers neither, so they read as null. The vendored API document is updated with both fields.
* `sreagent_repo_setting` takes `path_prefixes` (up to 20 directories a fix may change, `[]` returns to the directories detected from the repository's layout), `allow_ci_config`, `platform_fallback`, `draft_prs` and `check_network`. `path_prefixes` and `path_prefix` are two ways to state one fence: the platform derives each from the other (`path_prefix` answers the list's only entry and null for any other length) and refuses a write that names both, so a configuration that sets both is refused at validate and neither is carried over from the prior state when the resource changes. `draft_prs` unset follows the GitHub installation's setting, and removing it sends null to return to that. `check_network` and `allow_ci_config` widen what a fix's commands and CI may reach; both default to off on the platform. Needs a platform with the fields, which the live API already serves.
* `sreagent_github_settings` answers `stopped_from` and `stopped_note` (read only): the parent organization whose GitHub App a child organization stopped using, and the sentence the app shows for it. `sreagent_outbound_rule` answers `not_delivering` (read only): why a rule on a parent's paging target pages nothing after the organization stopped using the parent's targets.
* The descriptions of `fix_runner` and `ci_fallback_to_platform` follow the platform: `opencode_ci` is retired (`platform` and `self_hosted` remain) and `ci_fallback_to_platform` is stored and unused. The vendored API document is re-vendored whole, which also ends the daily contract drift failure that named `path_prefixes`, `allow_ci_config`, `platform_fallback`, `draft_prs`, `check_network` and `inheritance_stopped` as missing from the provider.

## [0.1.1]

FEATURES:

* `sreagent_ai_provider` takes `role_arn` and `region` for `provider_type = "bedrock"`: a Bedrock provider authenticates by assuming an IAM role in your own AWS account, whose trust policy uses the principal and ExternalId of `data.sreagent_aws_external_id`. Both are required for bedrock and refused for every other provider, a bedrock provider takes no `api_key_wo`, and the role ARN's shape, the region (one the platform supports, lowercase), and the match between the ARN's partition and the region are checked at plan; a value known only at apply (a role created in the same apply) is not judged. Removing both, or switching the provider away from bedrock, clears them on the platform; switching a provider to bedrock discards the API key it held, and the plan shows `api_key_set` becoming false. Needs a platform with the Bedrock role columns (v0.439.0 or later); an older platform refuses the fields. The vendored API document is re-vendored with both fields.

## [0.1.0]

The first version published to the Terraform Registry.

FEATURES:

* 21 collection resources with a full lifecycle: alert routes, outbound configs and rules, data sources,
  connectors, synthetic checks, SLIs, SLOs, deploy policies, alert mutes, certificate monitors, image targets,
  prompt templates, teams, team members, repo settings, service bindings, status page components, AI providers,
  ticket integrations and ticket import rules.
* 8 organization singletons, adopted on create and forgotten on destroy.
* 33 data sources: a list data source per collection (with `truncated` for capped lists), one per singleton,
  `sreagent_aws_external_id` for an IAM role's trust policy and `sreagent_request_signing` for the
  organization's request signing key ids and states.
* Secrets are write-only arguments (`<name>_wo`, `<name>_wo_version`, `<name>_set`); none is ever stored in
  state or plan.
* Every update and delete carries the row's version, so a change made in the app after the last refresh is
  refused (412) instead of overwritten.
* A plan-only mode for CI with an `api:config_read` key.
* The `organization` pin refuses a key minted for another organization before anything is written.
* `sreagent_ticket_integration` takes `auto_file`: every card created from then on is filed into the system without a button press.
* `sreagent_connector`: `connector_type = "aws_iam"` creates the IAM hygiene connector self-healing's Security area deactivates an unused access key through. Its role is built by the `aws-iam-hygiene` module of terraform-sre-agent; no runbook step can use the connector. The vendored API document is re-vendored with the description of the type.
* `data.sreagent_request_signing`: the organization's request signing key ids with their state and times, the hosts
  its configuration sends requests to with whether each is signed, `remote_locations_pending` (true while a check
  that runs from a remote location has not yet had its signature confirmed), the header format and the address of the
  verification guide. It never answers a key (reveal it on Settings, Request signing) and there is no resource: a
  key is rotated on the Settings page or with the MCP tools. The vendored API document is re-vendored with the
  `request_signing` resource and the description of the `aws_cleanup` connector type. Release this version after the
  platform release that serves `/api/v1/config/request_signing` (v0.395.0): a platform that does not (it answers 404) fails the
  read with an error naming the release it needs (v0.395.0 or later), and `configured` is always true when the read succeeds.
* `sreagent_synthetic_check`: a change to `config` is applied while the check sends headers or a body set through the API; the platform keeps them, so the refusal that guarded them is gone.
* `sreagent_service_binding` takes `log_inline_fields`: up to three JSON log fields shown inline on the live stream and in the Explorer for the service, on the binding with no environment; removing it, or writing `[]`, returns to the built-in choice (the platform stores `[]` as null, and the provider treats the two as one value). Names are kept exactly as written; more than three, a blank or over-long name, a control character, a repeat, and a binding that has an `environment` are refused at plan. Setting or clearing `log_inline_fields` needs platform v0.397.0 or later; an older platform refuses the call (422, `set_service_binding does not take "log_inline_fields"`). A binding that never sets it sends nothing for it, so every other update still works on an older platform; `account_id` and `region` are sent the same way, only to set a value or to clear one the state held. The vendored API document is re-vendored with the column.
* The contract test holds a read-only singleton (`request_signing`, `aws_external_id`) to every computed field the API document publishes, by kind, and holds a list's `MaxItems` to the published `maxItems`. A read of `data.sreagent_request_signing` on a platform that predates it also shows the platform's own sentence.

BREAKING CHANGES:

* `sreagent_outbound_config`: `base_url` is replaced by the write-only `base_url_wo` with `base_url_wo_version`,
  because a webhook URL carries its own token. The platform no longer answers the URL: a read answers the computed
  `base_url_set` and `base_url_host` (scheme, host and a non-default port, never the path or query). Leaving
  `base_url_wo` out keeps the stored URL.
* Removing the URL: remove both `base_url_wo` and `base_url_wo_version` after they were set, and the next apply
  sends `base_url: null` and plans `base_url_set = false`. This is asymmetric with the other secrets on purpose:
  removing `api_key_wo` and `api_key_wo_version` (or any other secret's pair) keeps the stored value, and
  `base_url_wo` alone cannot clear. A row imported or adopted without a recorded `base_url_wo_version` is never
  cleared by leaving the pair out.

UPGRADE NOTES:

* If you adopted an earlier export, replace `base_url = var.<label>_base_url` and its `variable` block with
  `base_url_wo = var.<label>_base_url` and `base_url_wo_version = 1`. Keep the variable and mark it
  `sensitive = true`, or take the value from an ephemeral resource, then apply once. The export no longer writes a
  `variable` block or a `base_url` line; it writes the commented `base_url_wo` and `base_url_wo_version` pair.
