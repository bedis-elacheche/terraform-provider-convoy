action "convoy_send_event" "ping_acme" {
  config {
    endpoint_id = convoy_endpoint.acme.id
    event_type  = "ping"
    data        = jsonencode({ message = "endpoint provisioned" })
  }
}

# Send a ping as soon as the endpoint is created...
resource "terraform_data" "acme_smoke_test" {
  input = convoy_endpoint.acme.id

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.convoy_send_event.ping_acme]
    }
  }
}

# ...or on demand: terraform apply -invoke=action.convoy_send_event.ping_acme
