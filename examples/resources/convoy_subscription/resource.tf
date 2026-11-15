resource "convoy_subscription" "acme_invoices" {
  name        = "acme-invoices"
  endpoint_id = convoy_endpoint.acme.id

  filter_config = {
    event_types = ["invoice.paid", "invoice.payment_failed"]
    filter = {
      body = jsonencode({
        data = {
          amount   = { "$gte" = 1000 }
          currency = { "$in" = ["usd", "eur"] }
        }
      })
    }
  }

  retry_config = {
    type        = "exponential"
    duration    = 30
    retry_count = 10
  }

  alert_config = {
    count     = 5
    threshold = "1h"
  }
}
