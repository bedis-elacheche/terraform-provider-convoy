# Requires a personal API key and the organisation ID
# (provider "organisation_id" or CONVOY_ORGANISATION_ID).
resource "convoy_project" "billing" {
  name = "billing"
  type = "outgoing"

  config = {
    disable_endpoint = true

    strategy = {
      type        = "exponential"
      duration    = 10
      retry_count = 8
    }

    signature = {
      header = "X-Convoy-Signature"
      versions = [
        { hash = "SHA256", encoding = "hex" },
      ]
    }

    meta_event = {
      is_enabled = true
      type       = "http"
      url        = "https://ops.example.com/convoy-meta"
      event_type = ["endpoint.created", "endpoint.disabled", "eventdelivery.failed"]
    }
  }
}

# The project API key is only returned at creation time.
output "billing_api_key" {
  value     = convoy_project.billing.api_key
  sensitive = true
}
