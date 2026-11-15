package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoytest"
)

func val(addr string, p tfjsonpath.Path, v knownvalue.Check) statecheck.StateCheck {
	return statecheck.ExpectKnownValue(addr, p, v)
}

func tfpath(parts ...any) tfjsonpath.Path {
	p := tfjsonpath.New(parts[0].(string))
	for _, x := range parts[1:] {
		switch k := x.(type) {
		case string:
			p = p.AtMapKey(k)
		case int:
			p = p.AtSliceIndex(k)
		}
	}
	return p
}

func TestProjectResource(t *testing.T) {
	s := newFakeConvoy(t)
	addr := "convoy_project.test"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(s) + `
resource "convoy_project" "test" {
  name = "billing"
  type = "outgoing"
  config = {
    strategy = { type = "exponential", retry_count = 5 }
    meta_event = {
      is_enabled = true
      type       = "http"
      url        = "https://meta.example.com"
      secret     = "s3cr3t"
      event_type = ["endpoint.created"]
    }
  }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("organisation_id"), knownvalue.StringExact(testOrgID)),
					val(addr, tfpath("api_key"), knownvalue.StringRegexp(regexp.MustCompile(`^CO\.`))),
					val(addr, tfpath("config", "strategy", "type"), knownvalue.StringExact("exponential")),
					val(addr, tfpath("config", "strategy", "retry_count"), knownvalue.Int64Exact(5)),
					// Omitted nested values come from Convoy's defaults.
					val(addr, tfpath("config", "strategy", "duration"), knownvalue.Int64Exact(10)),
					val(addr, tfpath("config", "ratelimit", "count"), knownvalue.Int64Exact(1000)),
					val(addr, tfpath("config", "signature", "versions", 0, "hash"), knownvalue.StringExact("SHA256")),
				},
			},
			{
				Config: providerConfig(s) + `
resource "convoy_project" "test" {
  name = "billing-v2"
  type = "outgoing"
  config = {
    disable_endpoint = true
    signature = {
      header = "X-Convoy-Signature"
      versions = [
        { hash = "SHA256", encoding = "hex" },
        { hash = "SHA512", encoding = "base64" },
      ]
    }
  }
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
				}},
				Check: func(*terraform.State) error {
					// The pre-existing signature version keeps its identity.
					p := s.Get("projects", "proj_0001")
					v := p["config"].(map[string]any)["signature"].(map[string]any)["versions"].([]any)
					if uid := v[0].(map[string]any)["uid"]; uid != "sigv1" {
						return fmt.Errorf("first signature version uid changed to %v", uid)
					}
					return nil
				},
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("name"), knownvalue.StringExact("billing-v2")),
					val(addr, tfpath("config", "disable_endpoint"), knownvalue.Bool(true)),
					// Values set in the previous step are kept when omitted.
					val(addr, tfpath("config", "strategy", "type"), knownvalue.StringExact("exponential")),
					val(addr, tfpath("config", "signature", "versions", 1, "hash"), knownvalue.StringExact("SHA512")),
				},
			},
			{
				ResourceName:            addr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"api_key"},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if n := s.Count("projects"); n != 2 {
				return fmt.Errorf("expected only the 2 seeded projects to remain, found %d", n)
			}
			return nil
		},
	})
}

func TestEndpointResource(t *testing.T) {
	s := newFakeConvoy(t)
	addr := "convoy_endpoint.test"
	var endpointID string
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(s) + `
resource "convoy_endpoint" "test" {
  name          = "acme"
  url           = "https://acme.example.com/webhooks"
  owner_id      = "acme"
  support_email = "ops@acme.example.com"
  secret        = "first-secret"
  paused        = true
  authentication = {
    type    = "api_key"
    api_key = { header_name = "X-Api-Key", header_value = "k1" }
  }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("project_id"), knownvalue.StringExact(testOutgoingID)),
					val(addr, tfpath("status"), knownvalue.StringExact("paused")),
					val(addr, tfpath("http_timeout"), knownvalue.Int64Exact(10)),
					val(addr, tfpath("content_type"), knownvalue.StringExact("application/json")),
					val(addr, tfpath("advanced_signatures"), knownvalue.Bool(true)),
				},
				Check: func(st *terraform.State) error {
					endpointID = st.RootModule().Resources[addr].Primary.ID
					return nil
				},
			},
			{
				// Rotate the secret, resume deliveries, switch auth, drop support_email.
				Config: providerConfig(s) + `
resource "convoy_endpoint" "test" {
  name              = "acme"
  url               = "https://acme.example.com/webhooks"
  owner_id          = "acme"
  secret            = "second-secret"
  secret_expiration = 3600
  paused            = false
  http_timeout      = 30
  authentication = {
    type       = "basic_auth"
    basic_auth = { username = "convoy", password = "p4ss" }
  }
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
				}},
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("status"), knownvalue.StringExact("active")),
					val(addr, tfpath("secret"), knownvalue.StringExact("second-secret")),
					val(addr, tfpath("http_timeout"), knownvalue.Int64Exact(30)),
					val(addr, tfpath("support_email"), knownvalue.Null()),
				},
				Check: func(*terraform.State) error {
					e := s.Get("endpoints", endpointID)
					if n := len(e["secrets"].([]any)); n != 2 {
						return fmt.Errorf("expected secret rotation to keep 2 secrets, got %d", n)
					}
					if e["support_email"] != "" {
						return fmt.Errorf("support_email was not cleared: %v", e["support_email"])
					}
					return nil
				},
			},
			{
				// OAuth2: Convoy encrypts client_secret and defaults grant_type;
				// neither must cause a diff.
				Config: providerConfig(s) + `
resource "convoy_endpoint" "test" {
  name     = "acme"
  url      = "https://acme.example.com/webhooks"
  owner_id = "acme"
  secret   = "second-secret"
  authentication = {
    type = "oauth2"
    oauth2 = {
      url                 = "https://auth.acme.example.com/token"
      client_id           = "convoy"
      client_secret       = "shh"
      authentication_type = "shared_secret"
    }
  }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("authentication", "oauth2", "client_secret"), knownvalue.StringExact("shh")),
					val(addr, tfpath("authentication", "oauth2", "grant_type"), knownvalue.Null()),
				},
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				// The OAuth2 secret is stored encrypted by Convoy, and secret_expiration is write-only.
				ImportStateVerifyIgnore: []string{"secret_expiration", "authentication.oauth2.client_secret", "authentication.oauth2.grant_type"},
			},
			{
				// Out-of-band deletion is detected and the endpoint recreated.
				PreConfig: func() { s.Delete("endpoints", endpointID) },
				Config: providerConfig(s) + `
resource "convoy_endpoint" "test" {
  name = "acme"
  url  = "https://acme.example.com/webhooks"
}`,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate),
				}},
			},
		},
	})
}

func TestEndpointResource_validation(t *testing.T) {
	s := newFakeConvoy(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{{
			Config: providerConfig(s) + `
resource "convoy_endpoint" "test" {
  name = "x"
  url  = "https://x.example.com"
  authentication = { type = "api_key" }
}`,
			ExpectError: regexp.MustCompile(`authentication.api_key must be set`),
		}},
	})
}

func TestSourceResource(t *testing.T) {
	s := newFakeConvoy(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig(s) + `
resource "convoy_source" "github" {
  project_id = "` + testIncomingID + `"
  name       = "github"
  type       = "http"
  verifier = {
    type = "hmac"
    hmac = { header = "X-Hub-Signature-256", hash = "SHA256", secret = "gh", encoding = "hex" }
  }
  idempotency_keys = ["request.headers.X-GitHub-Delivery"]
}

resource "convoy_source" "plain" {
  project_id = "` + testIncomingID + `"
  name       = "plain"
  type       = "http"
  is_disabled = true
}

resource "convoy_source" "rabbit" {
  name = "rabbit"
  type = "pub_sub"
  pub_sub = {
    type    = "amqp"
    workers = 2
    amqp = {
      schema        = "amqp"
      host          = "rabbit.internal"
      port          = "5672"
      queue         = "events"
      username      = "guest"
      password      = "guest"
      bind_exchange = "convoy"
      routing_key   = "events.#"
    }
  }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val("convoy_source.github", tfpath("url"), knownvalue.StringRegexp(regexp.MustCompile(`^https://ingest`))),
					val("convoy_source.plain", tfpath("verifier"), knownvalue.Null()),
					val("convoy_source.plain", tfpath("is_disabled"), knownvalue.Bool(true)),
					val("convoy_source.rabbit", tfpath("pub_sub", "amqp", "routing_key"), knownvalue.StringExact("events.#")),
				},
			},
			{
				ResourceName: "convoy_source.github",
				ImportState:  true,
				ImportStateIdFunc: func(st *terraform.State) (string, error) {
					return testIncomingID + "/" + st.RootModule().Resources["convoy_source.github"].Primary.ID, nil
				},
				ImportStateVerify: true,
			},
			{
				// Bare IDs fall back to the provider's default project.
				ResourceName:      "convoy_source.rabbit",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestSubscriptionResource(t *testing.T) {
	s := newFakeConvoy(t)
	addr := "convoy_subscription.test"
	base := providerConfig(s) + `
resource "convoy_endpoint" "ep" {
  name = "acme"
  url  = "https://acme.example.com/webhooks"
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				// Convoy stores the filter flattened ("data.amount"); the nested
				// document must not produce a diff (the framework re-plans
				// after apply and fails on a non-empty plan).
				Config: base + `
resource "convoy_subscription" "test" {
  name        = "acme-invoices"
  endpoint_id = convoy_endpoint.ep.id
  filter_config = {
    event_types = ["invoice.paid", "invoice.failed"]
    filter = {
      body = jsonencode({ data = { amount = { "$gte" = 100 }, currency = "usd" } })
    }
  }
  retry_config = { type = "exponential", retry_count = 7 }
  alert_config = { count = 10 }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("type"), knownvalue.StringExact("api")),
					val(addr, tfpath("delivery_mode"), knownvalue.StringExact("at_least_once")),
					val(addr, tfpath("retry_config", "type"), knownvalue.StringExact("exponential")),
					val(addr, tfpath("retry_config", "duration"), knownvalue.Int64Exact(10)),
					val(addr, tfpath("alert_config", "threshold"), knownvalue.StringExact("1h")),
					val(addr, tfpath("filter_config", "filter", "headers"), knownvalue.Null()),
				},
			},
			{
				Config: base + `
resource "convoy_subscription" "test" {
  name        = "acme-invoices"
  endpoint_id = convoy_endpoint.ep.id
  filter_config = {
    event_types = ["*"]
  }
  rate_limit_config = { count = 50, duration = 1 }
}`,
				ConfigStateChecks: []statecheck.StateCheck{
					val(addr, tfpath("filter_config", "filter"), knownvalue.Null()),
					val(addr, tfpath("rate_limit_config", "count"), knownvalue.Int64Exact(50)),
					val(addr, tfpath("retry_config", "type"), knownvalue.StringExact("exponential")),
				},
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestSubscriptionFilterEventTypePortalLink(t *testing.T) {
	s := newFakeConvoy(t)
	cfg := func(desc string) string {
		return providerConfig(s) + fmt.Sprintf(`
resource "convoy_endpoint" "ep" {
  name     = "acme"
  url      = "https://acme.example.com/webhooks"
  owner_id = "acme"
}

resource "convoy_subscription" "sub" {
  name        = "acme"
  endpoint_id = convoy_endpoint.ep.id
}

resource "convoy_subscription_filter" "paid" {
  subscription_id = convoy_subscription.sub.id
  event_type      = "invoice.paid"
  body            = jsonencode({ customer = { tier = "gold" } })
}

resource "convoy_event_type" "paid" {
  name        = "invoice.paid"
  category    = "billing"
  description = %q
  json_schema = jsonencode({ type = "object", properties = { id = { type = "string" } } })
}

resource "convoy_portal_link" "acme" {
  name                = "acme portal"
  owner_id            = convoy_endpoint.ep.owner_id
  can_manage_endpoint = true
}
`, desc)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("An invoice was paid"),
				ConfigStateChecks: []statecheck.StateCheck{
					val("convoy_subscription_filter.paid", tfpath("enabled_at"), knownvalue.NotNull()),
					val("convoy_portal_link.acme", tfpath("auth_type"), knownvalue.StringExact("static_token")),
					val("convoy_portal_link.acme", tfpath("endpoint_count"), knownvalue.Int64Exact(1)),
					val("convoy_portal_link.acme", tfpath("url"), knownvalue.StringRegexp(regexp.MustCompile(`token=`))),
				},
			},
			{
				Config: cfg("An invoice was fully paid"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("convoy_event_type.paid", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("convoy_subscription_filter.paid", plancheck.ResourceActionNoop),
				}},
			},
			{
				ResourceName: "convoy_subscription_filter.paid",
				ImportState:  true,
				ImportStateIdFunc: func(st *terraform.State) (string, error) {
					r := st.RootModule().Resources["convoy_subscription_filter.paid"].Primary
					return r.Attributes["subscription_id"] + "/" + r.ID, nil
				},
				ImportStateVerify: true,
			},
			{ResourceName: "convoy_event_type.paid", ImportState: true, ImportStateVerify: true},
			{ResourceName: "convoy_portal_link.acme", ImportState: true, ImportStateVerify: true},
		},
		CheckDestroy: func(*terraform.State) error {
			if n := s.Count("portal_links"); n != 0 {
				return fmt.Errorf("portal link was not revoked")
			}
			for _, e := range []string{"filters", "subscriptions", "endpoints"} {
				if n := s.Count(e); n != 0 {
					return fmt.Errorf("%d %s left behind", n, e)
				}
			}
			// Event types cannot be deleted, only deprecated.
			for _, id := range eventTypeIDs(s) {
				if s.Get("event_types", id)["deprecated_at"] == nil {
					return fmt.Errorf("event type %s was not deprecated", id)
				}
			}
			return nil
		},
	})
}

func TestDataSourcesAndEphemeral(t *testing.T) {
	s := newFakeConvoy(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		Steps: []resource.TestStep{
			{
				Config: providerConfig(s) + `
resource "convoy_endpoint" "ep" {
  name = "acme"
  url  = "https://acme.example.com/webhooks"
}
resource "convoy_event_type" "a" { name = "a.created" }
resource "convoy_portal_link" "pl" {
  name      = "acme"
  endpoints = [convoy_endpoint.ep.id]
  auth_type = "refresh_token"
}
`,
			},
			{
				Config: providerConfig(s) + `
resource "convoy_endpoint" "ep" {
  name = "acme"
  url  = "https://acme.example.com/webhooks"
}
resource "convoy_event_type" "a" { name = "a.created" }
resource "convoy_portal_link" "pl" {
  name      = "acme"
  endpoints = [convoy_endpoint.ep.id]
  auth_type = "refresh_token"
}

data "convoy_project" "current" {}
data "convoy_endpoint" "by_name" { name = "acme" }
data "convoy_event_types" "all" { depends_on = [convoy_event_type.a] }

ephemeral "convoy_portal_link_token" "t" {
  portal_link_id = convoy_portal_link.pl.id
}
provider "echo" {
  data = ephemeral.convoy_portal_link_token.t.token
}
resource "echo" "token" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					val("data.convoy_project.current", tfpath("type"), knownvalue.StringExact("outgoing")),
					statecheck.CompareValuePairs("data.convoy_endpoint.by_name", tfjsonpath.New("id"),
						"convoy_endpoint.ep", tfjsonpath.New("id"), compareEqual{}),
					val("data.convoy_event_types.all", tfpath("names"), knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("a.created")})),
					val("echo.token", tfpath("data"), knownvalue.StringRegexp(regexp.MustCompile(`^refreshed-`))),
				},
			},
		},
	})
}

func TestActions(t *testing.T) {
	s := newFakeConvoy(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		Steps: []resource.TestStep{{
			Config: providerConfig(s) + `
resource "convoy_endpoint" "ep" {
  name = "acme"
  url  = "https://acme.example.com/webhooks"
}

action "convoy_send_event" "ping" {
  config {
    endpoint_id = convoy_endpoint.ep.id
    event_type  = "ping"
    data        = jsonencode({ hello = "world" })
  }
}

resource "terraform_data" "smoke" {
  input = convoy_endpoint.ep.id
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.convoy_send_event.ping]
    }
  }
}`,
			Check: func(*terraform.State) error {
				if len(s.Events) != 1 || s.Events[0]["event_type"] != "ping" {
					return fmt.Errorf("expected one ping event, got %v", s.Events)
				}
				return nil
			},
		}},
	})
}

func TestProviderConfig_missingAPIKey(t *testing.T) {
	t.Setenv("CONVOY_API_KEY", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{{
			Config:      `data "convoy_project" "p" { id = "x" }`,
			ExpectError: regexp.MustCompile(`Missing API key`),
		}},
	})
}

func TestProviderConfig_badAPIKey(t *testing.T) {
	s := newFakeConvoy(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "convoy" {
  api_key  = "wrong"
  base_url = %q
}
data "convoy_project" "p" { id = %q }`, s.BaseURL(), testOutgoingID),
			ExpectError: regexp.MustCompile(`returned 401: invalid API key`),
		}},
	})
}

func eventTypeIDs(s *convoytest.Server) []string {
	var ids []string
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("et_%04d", i)
		if s.Get("event_types", id) != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type compareEqual struct{}

func (compareEqual) CompareValues(values ...any) error {
	if len(values) == 2 && values[0] == values[1] {
		return nil
	}
	return fmt.Errorf("values differ: %v", values)
}
