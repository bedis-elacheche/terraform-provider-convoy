# The provider's default project.
data "convoy_project" "current" {}

output "project_type" {
  value = data.convoy_project.current.type
}
