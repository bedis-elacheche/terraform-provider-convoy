# End-to-end: a SaaS that sends webhooks to its customers.
#
#   export CONVOY_API_KEY=...            # personal API key
#   export CONVOY_ORGANISATION_ID=...
#   terraform apply

terraform {
  required_version = ">= 1.14"
  required_providers {
    convoy = {
      source = "bedis-elacheche/convoy"
    }
  }
}

provider "convoy" {}

variable "customers" {
  description = "Customers that receive webhooks, keyed by customer ID."
  type = map(object({
    url         = string
    event_types = list(string)
    email       = optional(string)
  }))
  default = {
    acme = {
      url         = "https://hooks.acme.example.com/convoy"
      event_types = ["invoice.paid", "invoice.payment_failed"]
      email       = "oncall@acme.example.com"
    }
    globex = {
      url         = "https://globex.example.com/webhooks"
      event_types = ["*"]
    }
  }
}

resource "convoy_project" "webhooks" {
  name = "customer-webhooks"
  type = "outgoing"

  config = {
    strategy = { type = "exponential", duration = 10, retry_count = 8 }
  }
}

locals {
  project_id = convoy_project.webhooks.id
  event_types = {
    "invoice.paid"           = "Sent when an invoice is fully paid."
    "invoice.payment_failed" = "Sent when a payment attempt fails."
  }
}

resource "convoy_event_type" "this" {
  for_each = local.event_types

  project_id  = local.project_id
  name        = each.key
  category    = "billing"
  description = each.value
}

resource "convoy_endpoint" "customer" {
  for_each = var.customers

  project_id    = local.project_id
  name          = each.key
  url           = each.value.url
  owner_id      = each.key
  support_email = each.value.email
}

resource "convoy_subscription" "customer" {
  for_each = var.customers

  project_id  = local.project_id
  name        = "${each.key}-default"
  endpoint_id = convoy_endpoint.customer[each.key].id

  filter_config = {
    event_types = each.value.event_types
  }
}

resource "convoy_portal_link" "customer" {
  for_each = var.customers

  project_id          = local.project_id
  name                = "${each.key} webhooks"
  owner_id            = convoy_endpoint.customer[each.key].owner_id
  can_manage_endpoint = true
}

# Smoke test every new endpoint.
action "convoy_send_event" "ping" {
  for_each = var.customers

  config {
    project_id  = local.project_id
    endpoint_id = convoy_endpoint.customer[each.key].id
    event_type  = "ping"
    data        = jsonencode({ hello = each.key })
  }
}

resource "terraform_data" "ping" {
  for_each = var.customers

  input = convoy_endpoint.customer[each.key].id

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.convoy_send_event.ping[each.key]]
    }
  }
}

output "portal_urls" {
  value     = { for k, v in convoy_portal_link.customer : k => v.url }
  sensitive = true
}

output "endpoint_secrets" {
  value     = { for k, v in convoy_endpoint.customer : k => v.secret }
  sensitive = true
}
