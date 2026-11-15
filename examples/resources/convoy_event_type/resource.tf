resource "convoy_event_type" "invoice_paid" {
  name        = "invoice.paid"
  category    = "billing"
  description = "Sent when an invoice is fully paid."
  json_schema = jsonencode({
    "$schema" = "https://json-schema.org/draft/2020-12/schema"
    type      = "object"
    required  = ["id", "amount"]
    properties = {
      id     = { type = "string" }
      amount = { type = "integer" }
    }
  })
}
