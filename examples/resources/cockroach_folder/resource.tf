resource "cockroach_folder" "a_team" {
  name      = "a-team"
  parent_id = "root"
}

resource "cockroach_folder" "a_team_dev" {
  name      = "dev"
  parent_id = cockroach_folder.a_team.id
}
