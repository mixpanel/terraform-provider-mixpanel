data "mixpanel_tag" "example" {
  project_id = 1234567
  tag_id     = 42
}

output "tag_name" {
  value = data.mixpanel_tag.example.name
}
