data "convoy_event_types" "all" {}

# Subscribe an endpoint to every non-deprecated event type.
resource "convoy_subscription" "everything" {
  name        = "acme-everything"
  endpoint_id = convoy_endpoint.acme.id
  filter_config = {
    event_types = data.convoy_event_types.all.names
  }
}
