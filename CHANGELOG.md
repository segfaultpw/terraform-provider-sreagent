## Unreleased

The first version, not yet published to a registry.

FEATURES:

* 21 collection resources with a full lifecycle: alert routes, outbound configs and rules, data sources,
  connectors, synthetic checks, SLIs, SLOs, deploy policies, alert mutes, certificate monitors, image targets,
  prompt templates, teams, team members, repo settings, service bindings, status page components, AI providers,
  ticket integrations and ticket import rules.
* 8 organization singletons, adopted on create and forgotten on destroy.
* 32 data sources: a list data source per collection (with `truncated` for capped lists), one per singleton,
  and `sreagent_aws_external_id` for an IAM role's trust policy.
* Secrets are write-only arguments (`<name>_wo`, `<name>_wo_version`, `<name>_set`); none is ever stored in
  state or plan.
* Every update and delete carries the row's version, so a change made in the app after the last refresh is
  refused (412) instead of overwritten.
* A plan-only mode for CI with an `api:config_read` key.
* The `organization` pin refuses a key minted for another organization before anything is written.
* `sreagent_ticket_integration` takes `auto_file`: every card created from then on is filed into the system without a button press.
* `sreagent_synthetic_check`: a change to `config` is applied while the check sends headers or a body set through the API; the platform keeps them, so the refusal that guarded them is gone.

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
