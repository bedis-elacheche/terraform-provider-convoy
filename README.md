# Terraform Provider for Convoy

Manage [Convoy](https://www.getconvoy.io) webhooks gateway resources with Terraform:
projects, endpoints, sources, subscriptions, subscription filters, event types and
portal links. It also provides data sources, an ephemeral resource and actions.

```hcl
terraform {
  required_providers {
    convoy = {
      source = "bedis-elacheche/convoy"
    }
  }
}

provider "convoy" {} # reads CONVOY_API_KEY, CONVOY_PROJECT_ID, CONVOY_BASE_URL

resource "convoy_endpoint" "acme" {
  name     = "acme"
  url      = "https://hooks.acme.example.com/convoy"
  owner_id = "customer_acme"
}

resource "convoy_subscription" "acme" {
  name        = "acme-invoices"
  endpoint_id = convoy_endpoint.acme.id
  filter_config = {
    event_types = ["invoice.paid"]
    filter      = { body = jsonencode({ data = { amount = { "$gte" = 1000 } } }) }
  }
}

resource "convoy_portal_link" "acme" {
  name     = "Acme webhooks"
  owner_id = convoy_endpoint.acme.owner_id
}
```

See [`examples/complete`](examples/complete/main.tf) for a multi-customer setup and
[`docs/`](docs/index.md) for the full reference, which is generated with `tfplugindocs`.

## What's covered

| Kind | Name | Convoy API |
|---|---|---|
| Resource | `convoy_project` | `/v1/projects` (personal API key + organisation ID) |
| Resource | `convoy_endpoint` | `/endpoints`, plus `pause` and `expire_secret` |
| Resource | `convoy_source` | `/sources` (HTTP ingest with verifiers, or SQS / Google Pub/Sub / Kafka / AMQP) |
| Resource | `convoy_subscription` | `/subscriptions` |
| Resource | `convoy_subscription_filter` | `/subscriptions/{id}/filters` |
| Resource | `convoy_event_type` | `/event-types` (destroy = deprecate) |
| Resource | `convoy_portal_link` | `/portal-links` (destroy = revoke) |
| Data source | `convoy_project`, `convoy_endpoint`, `convoy_source`, `convoy_event_types` | lookups by ID or name |
| Ephemeral resource | `convoy_portal_link_token` | `/portal-links/{id}/refresh_token` (Terraform 1.10+) |
| Action | `convoy_send_event`, `convoy_endpoint_secret_rotation`, `convoy_event_replay` | `/events`, `expire_secret`, `/events/{id}/replay` (Terraform 1.14+) |

Event deliveries, delivery attempts and meta events are run-time data rather than
infrastructure, so the provider doesn't model them as resources.

## Design choices (and how they follow the Stripe provider)

The [official Stripe provider](https://registry.terraform.io/providers/stripe/stripe/latest/docs)
was the reference. This provider follows it on these points:

- **Plugin Framework, protocol 6.** It uses `terraform-plugin-framework`, not SDKv2,
  and has the same package layout: `resources`, `datasources`, `ephemeralresources`
  and `actions`, plus a small API client in `internal/convoy`.
- **Minimal provider config with env fallbacks.** Stripe has `api_key` /
  `STRIPE_API_KEY` and an account context (`stripe_account`). This provider has
  `api_key` / `CONVOY_API_KEY` and a default `project_id` / `CONVOY_PROJECT_ID`, which
  plays the same role as the account context. It also has `base_url` (Convoy Cloud
  US or EU, or self-hosted), `organisation_id` and `max_retries`.
- **Resources mirror API objects 1:1.** Resource and attribute names follow the
  Convoy API field names (`owner_id`, `filter_config`, `rate_limit_duration`,
  `ratelimit` in project config, and so on), so the API reference maps straight onto
  HCL.
- **Read-only fields are `Computed` with `UseStateForUnknown`, immutable fields use
  `RequiresReplace`, and secrets are `Sensitive`.** The same goes for values that
  Convoy only returns once, such as the project `api_key`, which, like Stripe's
  webhook `secret`, is only returned at creation.
- **Imperative operations are actions, and short-lived credentials are ephemeral
  resources.** Stripe models refunds and payouts as actions and checkout sessions
  as ephemeral resources. Here, sending, replaying and secret rotation are actions,
  and portal tokens are ephemeral.
- **Everything is importable.** Import IDs are `<project_id>/<id>`, or just `<id>`
  when the provider has a default project.
- **Docs are generated** from the schema descriptions and `examples/` with
  `tfplugindocs`, using the same layout as Stripe's `docs/`.

These choices are Convoy-specific:

- **Nested objects are nested attributes** (`config = { strategy = { ... } }`), not
  blocks. HashiCorp recommends nested attributes for new framework providers. They
  also let objects that Convoy always fills with defaults be Optional+Computed.
- **Partial nested objects are merged with server values.** Convoy replaces nested
  config objects (strategy, ratelimit, retry_config, ...) as a whole. When you set
  only some of their fields, the provider fills in the rest from Convoy's current
  values, so the fields you leave out are not reset to zero.
- **Convoy rewrites some documents.** It flattens subscription filters
  (`{"data":{"x":1}}` becomes `{"data.x":1}`), encrypts OAuth2 client secrets and
  never returns mTLS private keys. The provider compares filters in a
  flattening-independent way and keeps the values you configured for write-only
  secrets, so none of this causes a perpetual diff.
- **Some API operations have no matching CRUD call.** Pausing is a toggle, so
  `paused` is a boolean that the provider reconciles. Changing `secret` calls
  `expire_secret` instead of replacing the endpoint. Event types can't be deleted,
  so destroy deprecates them, and portal links are revoked on destroy.
- **Features that depend on the Convoy edition** (support email, OAuth2, mTLS,
  transformations) are kept exactly as you configured them during apply. If your
  Convoy edition ignores one, it shows up as drift on the next plan instead of as a
  "provider produced inconsistent result" crash.

## Authentication

| Scope | Key | Can manage |
|---|---|---|
| Project | Project API key (*Project settings → Secrets*) | everything inside that project |
| Organisation | Personal API key (*Account → Personal access tokens*) | `convoy_project`, and resources in any project you can access |

## Development

```bash
make build      # go build
make test       # unit tests + end-to-end Terraform tests against an in-memory fake Convoy
make testacc    # acceptance tests against a real Convoy (needs CONVOY_API_KEY, CONVOY_PROJECT_ID)
make docs       # regenerate docs/ with tfplugindocs
```

`make test` runs real `terraform plan/apply/import/destroy` cycles against
[`internal/convoytest`](internal/convoytest/server.go). That's an in-memory fake of
the Convoy API that reproduces the server behaviours the provider has to handle
(defaults, flattened filters, the pause toggle, secret rotation, encrypted
secrets). The tests need a `terraform` binary, version 1.14 or later for actions,
either on `PATH` or set with `TF_ACC_TERRAFORM_PATH`.

To try a local build, add a `dev_overrides` entry for `bedis-elacheche/convoy` in
`~/.terraformrc` that points to your `$GOPATH/bin`, then run `make install`.

The registry address is `registry.terraform.io/bedis-elacheche/convoy`. This is a
community provider and is not affiliated with Frain Technologies, the makers of Convoy.

## License

[MIT](LICENSE)
