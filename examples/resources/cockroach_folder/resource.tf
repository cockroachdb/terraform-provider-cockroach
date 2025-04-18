resource "cockroach_folder" "a_team" {
  name      = "a-team"
  parent_id = "root"
  labels = {
    environment   = "staging",
    "cost-center" = "mkt-1234"
  }
}

resource "cockroach_folder" "a_team_dev" {
  name      = "dev"
  parent_id = cockroach_folder.a_team.id
  labels = {
    environment   = "production",
    "cost-center" = "mkt-5678"
  }
}
