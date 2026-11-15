data "convoy_source" "stripe" {
  project_id = var.incoming_project_id
  name       = "stripe"
}

output "stripe_ingest_url" {
  value = data.convoy_source.stripe.url
}
