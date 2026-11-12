---
page_title: "Provider: SRE Agent"
description: |-
  Manage SRE Agent configuration (alert routes, paging, synthetic checks, SLOs and the rest) as code.
---

# SRE Agent Provider

The `sreagent` provider manages an [SRE Agent](https://sreagent.app) organization's configuration through the
same configuration API (`/api/v1/config`) the app itself uses. Every write goes through the same checks, plan
gates and audit trail as the matching action in the browser.

It needs Terraform 1.12 or later: secrets are write-only arguments (Terraform 1.11), and service bindings import
by resource identity (Terraform 1.12). OpenTofu reads the same provider.

## Example Usage

```terraform
terraform {
  required_version = ">= 1.12"
  required_providers {
    sreagent = {
      source = "segfaultpw/sreagent"
    }
  }
}

variable "sreagent_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# An sre_ak_ key holding the api:admin scope applies changes. Pinning the
# organization makes a key minted for another organization fail before
# anything is written.
provider "sreagent" {
  api_key      = var.sreagent_api_key
  organization = "acme"
}
```

## Authentication

The provider authenticates with an organization API key (`sre_ak_...`). A personal token is refused before any
request is sent, because the configuration API only admits API keys.

- **Applying:** a key holding the `api:admin` scope. It can change every resource this provider manages, so
  mint one per workspace, give it an expiry, and pass it through an ephemeral variable or `SREAGENT_API_KEY`.
- **Planning only (CI):** a separate key holding `api:config_read`. That scope is a key of its own: it reads
  every resource and data source, so `terraform plan` works, and the platform refuses every create, update and
  delete. An apply with it fails with *"The configured API key holds api:config_read, which can plan but not
  apply. Apply with an api:admin key."*

```terraform
# A CI job that only runs terraform plan (pull request checks, drift
# reports) holds a separate key with the api:config_read scope. It reads every
# resource and data source, so plan works; any create, update or delete is
# refused, so apply fails with: "The configured API key holds
# api:config_read, which can plan but not apply."
#
# Set SREAGENT_API_KEY to the api:config_read key in the CI job's
# environment rather than naming it here.
provider "sreagent" {
  organization = "acme"
}
```

Both kinds need the organization's plan to include advanced integrations.

## The organization pin

Set `organization` to the organization's slug. Every answer names the organization the key belongs to, and when
it differs from the pin the provider stops before recording anything in state, so a key minted for one
organization can never apply a plan written for another.

## Transport and retries

`base_url` must use `https`; plain `http` is accepted only for `localhost` and `127.0.0.1`. A rate-limited
request (429) is retried on every method after the `retry-after` the platform names (at most 60 seconds). A
gateway error (502, 503, 504) or a dropped connection is retried on reads, updates and deletes, never on a
create, which has no key that would make a second attempt safe. `max_retries` bounds both (default 4).

## Changes made outside Terraform

Every update and delete carries the version of the row Terraform last read. When the row changed in the app
after that refresh (someone edited it in the browser), the platform answers 412 and nothing is written:

> sreagent_x changed outside Terraform after the last refresh (the platform answered 412). Nothing was written.
> Run terraform plan again to review the change.

Run `terraform plan` again, review what changed, and apply the new plan. Background work (probes, scans, the
scheduler) never moves that version, so it never causes a 412.

A row deleted in the app is recreated by the next plan. A singleton (`sreagent_organization_settings` and the
other settings resources) cannot be deleted: destroying it only stops Terraform managing it. The one exception is
`sreagent_repository_defaults` in a child organization, which hands the child back to its parent's defaults and is
planned like a change (see below).

## Changes the platform previews

A change to the organization's repository defaults (`sreagent_repository_defaults`) moves every repository that
follows them, so the platform applies it only against a preview of what it would move. `terraform plan` asks for
that preview and lists the settings that change, the repositories that read a different value and the child
organizations that follow the defaults; the apply sends the fingerprint of that preview and fails, writing
nothing, when what would move changed since. The preview needs an `api:admin` key. A plan made with an
`api:config_read` key says so, and the apply then previews the change itself and applies what it finds. Such a plan can only judge a confirmation against what the row holds, so for a child organization that still follows its parent it asks for the confirmation of a setting that is already on; add it, which is harmless. Terraform
plans a change again when it applies a saved plan, so the preview is requested at both steps and the second one
must agree with the first.

Destroying `sreagent_repository_defaults` in a child organization hands its repositories back to the parent's
defaults, so the destroy plan asks for the preview too: it warns with the settings and repositories that move
(and a stop the organization made in the app, which the destroy ends even when no value moves), and the delete uses the fingerprint of that plan; with an `api:config_read` plan key the delete previews itself and writes against that preview. When the parent's
defaults turn CI configuration or network access on, the platform wants a confirmation that a destroy cannot
carry: set `confirm_on_destroy`, apply, then destroy. The destroy plan fails without it and the provider never
confirms on its own. A destroy that moves nothing and ends no stop says nothing. If the platform refuses the destroy, or the plan cannot be made, the error names the ways to stop managing the defaults without changing them (a `removed` block with `lifecycle { destroy = false }`, or `terraform state rm`). In an organization with no parent the plan only warns that the defaults stay in force.

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `api_key` (String, Sensitive) An sre_ak_ API key: one holding api:admin to apply, or a separate api:config_read key for plan-only CI jobs. Defaults to SREAGENT_API_KEY.
- `base_url` (String) Platform URL. Defaults to SREAGENT_BASE_URL, then https://sreagent.app. Must be https except for localhost.
- `max_retries` (Number) Retries for rate limiting and gateway errors, 0 to 10. Defaults to 4.
- `organization` (String) The organization slug this configuration belongs to. When set, every answer must name it, so a key for another organization can never apply this plan. Defaults to SREAGENT_ORGANIZATION.
