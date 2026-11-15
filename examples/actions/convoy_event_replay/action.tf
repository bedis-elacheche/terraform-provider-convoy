# terraform apply -invoke=action.convoy_event_replay.incident_1234 -var event_id=...
action "convoy_event_replay" "incident_1234" {
  config {
    event_id = var.event_id
  }
}

variable "event_id" {
  type = string
}
