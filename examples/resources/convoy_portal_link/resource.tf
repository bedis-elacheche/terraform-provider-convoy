resource "convoy_portal_link" "acme" {
  name                = "Acme webhooks portal"
  owner_id            = convoy_endpoint.acme.owner_id
  can_manage_endpoint = true
}

output "acme_portal_url" {
  value     = convoy_portal_link.acme.url
  sensitive = true
}
