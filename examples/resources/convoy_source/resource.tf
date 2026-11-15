# Incoming project: receive GitHub webhooks and verify their signature.
resource "convoy_source" "github" {
  project_id = var.incoming_project_id
  name       = "github"
  type       = "http"

  verifier = {
    type = "hmac"
    hmac = {
      header   = "X-Hub-Signature-256"
      hash     = "SHA256"
      encoding = "hex"
      secret   = var.github_webhook_secret
    }
  }

  idempotency_keys = ["request.headers.X-GitHub-Delivery"]
}

# Give this URL to GitHub.
output "github_ingest_url" {
  value = convoy_source.github.url
}

# Outgoing project: consume events from an SQS queue.
resource "convoy_source" "orders_queue" {
  name = "orders"
  type = "pub_sub"

  pub_sub = {
    type    = "sqs"
    workers = 4
    sqs = {
      queue_name     = "orders-events"
      default_region = "eu-west-1"
      access_key_id  = var.aws_access_key_id
      secret_key     = var.aws_secret_access_key
    }
  }
}

variable "incoming_project_id" { type = string }
variable "github_webhook_secret" {
  type      = string
  sensitive = true
}
variable "aws_access_key_id" { type = string }
variable "aws_secret_access_key" {
  type      = string
  sensitive = true
}
