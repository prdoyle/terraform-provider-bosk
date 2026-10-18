# Terraform Provider for bosk

Manages nodes in a [bosk](https://github.com/boskworks/bosk) state tree through the
bosk maintenance HTTP API.

> **Warning**
> The maintenance endpoints expose full read and write access to the state tree.
> Enable them deliberately, require authentication
> (`bosk.web.maintenance.access=AUTHENTICATED`), and use a credential with the
> smallest appropriate authority, such as `bosk:state`. Do not expose them to
> untrusted networks.

## Requirements

- A bosk application using `bosk-spring-boot` with the maintenance endpoints enabled.
- Terraform 1.13 or later.
- Go 1.26 or later to build from source.

## Usage

```hcl
terraform {
  required_providers {
    bosk = {
      source = "prdoyle/bosk"
    }
  }
}

provider "bosk" {
  endpoint = "https://app.example.com/bosk/state"
  token    = var.bosk_token
}

resource "bosk_node" "world" {
  path = "/targets/world"
  value = jsonencode({
    id   = "world"
    name = "World"
  })
}
```

## Provider configuration

| Attribute | Environment variable | Description |
|---|---|---|
| `endpoint` | `BOSK_ENDPOINT` | Base URL of the maintenance endpoints. |
| `token` | `BOSK_TOKEN` | Bearer token. |
| `username` | `BOSK_USERNAME` | HTTP Basic username. |
| `password` | `BOSK_PASSWORD` | HTTP Basic password. |
| `headers` | | Additional HTTP headers. |
| `request_timeout` | `BOSK_REQUEST_TIMEOUT` | Per-request timeout; defaults to `10s`. |
| `ca_cert` | `BOSK_CA_CERT` | PEM-encoded CA certificate. |
| `insecure_skip_verify` | `BOSK_INSECURE_SKIP_VERIFY` | Skip TLS verification; local development only. |

Set either `token` or `username`/`password`, not both.

### Managing multiple bosks

Each provider configuration targets one maintenance endpoint. Use provider
aliases to manage more than one bosk in the same configuration:

```hcl
provider "bosk" {
  endpoint = "https://prod.example.com/bosk/state"
  token    = var.prod_token
}

provider "bosk" {
  alias    = "staging"
  endpoint = "https://staging.example.com/bosk/state"
  token    = var.staging_token
}

resource "bosk_node" "prod_world" {
  path  = "/targets/world"
  value = jsonencode({ id = "world", name = "World" })
}

resource "bosk_node" "staging_world" {
  provider = bosk.staging
  path     = "/targets/world"
  value    = jsonencode({ id = "world", name = "World" })
}
```

Resources and data sources select a configuration with
`provider = bosk.<alias>`. If you prefer one state per environment, use
Terraform workspaces or separate root modules instead.

## `bosk_node`

`path` is the node's identity, relative to the maintenance endpoint. Use
`jsonencode(...)` for `value`.

- **Create** fails if the node already exists. Import it first to adopt an
  existing node.
- **Update** replaces the value. If bosk returns an ETag (a node with a
  `revision` field), the provider sends it in `If-Match`, so a concurrent change
  is rejected rather than silently overwritten.
- **Delete** removes the node. bosk rejects deleting required fields.
- **Read** sends `Cache-Control: no-cache`, which makes bosk flush before
  reading.
- Every write is followed by a fresh read. If the value does not match what
  Terraform asked for, the provider reports a concurrency or write failure
  instead of recording false success.

For nodes that have a `revision` field, include it in `value`; the provider
treats it as opaque and uses the ETag for conditional writes.

Nodes without a `revision` field have no ETag, so updates are last-writer-wins.

## Import

```shell
terraform import bosk_node.world /targets/world
```

Terraform 1.12 and later can also import by resource identity:

```hcl
import {
  to = bosk_node.world
  identity = {
    path = "/targets/world"
  }
}
```

## Security

- The maintenance endpoints expose full read and write access to the state
  tree. Enable them with `bosk.web.maintenance.access=AUTHENTICATED`, require
  TLS, and use a credential with the smallest appropriate authority (for
  example `bosk:state`).
- The provider verifies TLS certificates by default. Prefer `ca_cert` for a
  private CA. Use `insecure_skip_verify` only for local development; the
  provider warns when it is enabled, and it cannot be combined with `ca_cert`.
- Credentials are marked sensitive and are not logged. They can still appear
  in Terraform variables, saved plans, and CI configuration, so protect those
  artifacts.
- Node values are stored in Terraform state. Error diagnostics can include
  observed node values and bosk response bodies (truncated). Use encrypted
  remote state and restrict access to plans, state, and logs. Do not put
  secrets in nodes unless you have accepted those exposures.
- Client certificates (mTLS) are not yet supported. Server-side certificates,
  private CAs, and custom headers are supported.
- Report suspected vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## Development

```shell
make build
make test
make testacc
make generate
```

Acceptance tests use an in-process fake maintenance server and do not require a
running bosk application. The Terraform CLI is downloaded automatically by the
testing framework if it is not already installed.

## Previous implementation

The pre-rewrite implementation is preserved at tag `v0.0.16`.

## License

MPL-2.0. See [LICENSE](LICENSE).
