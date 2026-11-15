data "convoy_endpoint" "legacy" {
  name = "legacy-receiver"
}

resource "convoy_subscription" "legacy_all" {
  name        = "legacy-all-events"
  endpoint_id = data.convoy_endpoint.legacy.id
}
