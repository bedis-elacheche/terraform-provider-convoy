# Only deliver invoice.paid events for gold customers, without affecting the
# other event types of the subscription.
resource "convoy_subscription_filter" "gold_invoices" {
  subscription_id = convoy_subscription.acme_invoices.id
  event_type      = "invoice.paid"
  body = jsonencode({
    customer = { tier = "gold" }
  })
}
