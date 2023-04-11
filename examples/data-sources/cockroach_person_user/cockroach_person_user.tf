variable "email_address" {
  type = string
}

data "cockroach_person_user" "cockroach" {
  email = var.email_address
}
