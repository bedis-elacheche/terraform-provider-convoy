resource "convoy_endpoint" "acme" {
  name          = "acme-production"
  url           = "https://hooks.acme.example.com/convoy"
  owner_id      = "customer_acme"
  description   = "Acme Corp production receiver"
  support_email = "oncall@acme.example.com"
  http_timeout  = 15

  # Changing secret later rotates it through Convoy's expire-secret API.
  secret            = var.acme_webhook_secret
  secret_expiration = 3600

  authentication = {
    type = "api_key"
    api_key = {
      header_name  = "X-Acme-Token"
      header_value = var.acme_token
    }
  }
}

variable "acme_webhook_secret" {
  type      = string
  sensitive = true
}

variable "acme_token" {
  type      = string
  sensitive = true
}
