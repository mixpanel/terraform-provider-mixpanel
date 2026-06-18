data "mixpanel_project_outgoing_integration" "example" {
  project_id                      = 1234567
  project_outgoing_integration_id = 98765
}

output "integration_name" {
  value = data.mixpanel_project_outgoing_integration.example.name
}
