# Import a property_definition by "<project_id>:<resource_type>:<name>".
# resource_type is "Event" or "User". The property name comes last so keys
# containing ":" import correctly.
terraform import mixpanel_property_definition.plan_type "1234567:Event:plan_type"
terraform import mixpanel_property_definition.email '1234567:User:$email'
