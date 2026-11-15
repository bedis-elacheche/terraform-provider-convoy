# terraform apply -invoke=action.convoy_endpoint_secret_rotation.acme
action "convoy_endpoint_secret_rotation" "acme" {
  config {
    endpoint_id = convoy_endpoint.acme.id
    expiration  = 3600
  }
}
