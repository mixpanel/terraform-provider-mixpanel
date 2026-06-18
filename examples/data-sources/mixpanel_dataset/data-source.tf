data "mixpanel_dataset" "events" {
  project_id = 1234567
  dataset_id = "my_dataset"
}

output "dataset_name" {
  value = data.mixpanel_dataset.events.name
}
