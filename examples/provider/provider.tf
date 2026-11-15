terraform {
  required_providers {
    convoy = {
      source = "bedis-elacheche/convoy"
    }
  }
}

# Credentials are read from CONVOY_API_KEY when api_key is not set.
provider "convoy" {
  base_url   = "https://us.getconvoy.cloud/api" # or https://eu.getconvoy.cloud/api, or your self-hosted URL
  project_id = var.convoy_project_id            # default project for project-scoped resources
}

variable "convoy_project_id" {
  type = string
}
