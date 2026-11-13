# terraform-provider-sreagent
Terraform and OpenTofu provider for SRE Agent (sreagent.app): manage alert routes, outbound paging, synthetic checks, SLOs and the rest of an organization's configuration as code.

```terraform
terraform {
  required_providers {
    sreagent = {
      source = "segfaultpw/sreagent"
    }
  }
}

provider "sreagent" {
  organization = "acme" # the API key comes from SREAGENT_API_KEY
}

resource "sreagent_alert_route" "checkout" {
  service = "checkout"
  target  = "#checkout-alerts"
}
```

The provider is published on the Terraform Registry as `segfaultpw/sreagent` (OpenTofu reads it from the same
address). The documentation lives in [docs/](docs/index.md), generated from [templates/](templates) and
[examples/](examples) by `go generate ./...`.

## Platform releases

Most of the provider works against any platform that serves the configuration API. These parts need a release:

| Part | Platform release |
| --- | --- |
| `sreagent_repository_defaults`, `data.sreagent_repository_defaults` | v0.477.0 |
| `follows_default`, `set_here` and `following` on `sreagent_repo_setting` | v0.482.0 |
| `data.sreagent_effective_repo_settings` | v0.477.0 |
| The security, code detection and allow rule data sources | the release that first served each, named in the error an older platform answers |

On a platform from before a part, the provider says which release the part needs; nothing else is affected.

## How it works

The provider talks to the platform's configuration API (`/api/v1/config`), the same door the app's MCP tools use.
One generic resource and data source (`internal/engine`) read a declarative spec per API resource
(`internal/specs`). `internal/contract` holds every spec to the API's OpenAPI document, vendored from
sre-agent's committed snapshot; a daily job compares production's document with it by shape (a reworded
description is not drift) and re-vendors it when anything differs, failing only when the tests do not pass against it.

## Development

Needs Go (the version in `go.mod`) and Terraform 1.12 or later on `PATH`.

```shell
gofmt -l .                 # prints nothing
golangci-lint run ./...    # 0 issues
go test ./...              # unit tests, against an in-memory fake of the API
go generate ./...          # regenerates docs/
terraform fmt -check -recursive examples
```

The CI workflow runs the same checks on Terraform 1.12.2 (the supported floor) and the latest release.

Acceptance tests run against a throwaway local stack, never production:

```shell
./acceptance/seed.sh > /tmp/tf-acc.env   # starts Postgres and the pinned sre-agent image, seeds an organization
set -a; . /tmp/tf-acc.env; set +a
TF_ACC=1 go test ./internal/provider/ -run TestAcc -timeout 60m
docker compose -f acceptance/docker-compose.yml down -v
```

`acceptance/IMAGE` holds the short commit SHA (one line, no tag prefix, for example `8f621a6`) of the sre-agent image the
stack runs, pulled from `ghcr.io/segfaultpw/sre-agent`. Set it to the tag production serves when a release is cut, so the
acceptance run exercises the platform the provider ships against.
