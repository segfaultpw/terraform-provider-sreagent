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
