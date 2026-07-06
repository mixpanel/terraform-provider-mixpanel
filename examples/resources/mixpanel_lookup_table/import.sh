# Import a lookup_table by "<project_id>:<data_group_id>".
# The data-group id is a full-range int64 and may be negative; everything
# after the first ":" is the id. After import, the first apply re-uploads
# the configured csv_content (the API cannot echo the stored CSV back).
terraform import mixpanel_lookup_table.accounts "1234567:-9199707515373727904"
