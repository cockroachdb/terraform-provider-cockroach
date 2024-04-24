resource "cockroach_api_key" "example" {
  name               = "An example api key"
  service_account_id = cockroach_service_account.example_sa.id
}

output "example_secret" {
  value       = cockroach_api_key.example.secret
  description = "The api key for the example api key"
  sensitive   = true
}
