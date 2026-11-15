resource "convoy_portal_link" "acme" {
  name      = "Acme portal"
  owner_id  = "customer_acme"
  auth_type = "refresh_token"
}

# A fresh token on every run, never written to plan or state.
ephemeral "convoy_portal_link_token" "acme" {
  portal_link_id = convoy_portal_link.acme.id
}

# Hand it to a write-only argument, e.g. a secret store.
resource "aws_secretsmanager_secret_version" "acme_portal" {
  secret_id                = "acme/convoy-portal-token"
  secret_string_wo         = ephemeral.convoy_portal_link_token.acme.token
  secret_string_wo_version = 1
}
