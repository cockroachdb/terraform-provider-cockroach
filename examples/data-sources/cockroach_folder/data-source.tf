variable "prod_folder_id" {
  type = string
}

data "cockroach_folder" "team1" {
  path = "/prod/team1"
}

data "cockroach_folder" "prod" {
  id = var.prod_folder_id
}
