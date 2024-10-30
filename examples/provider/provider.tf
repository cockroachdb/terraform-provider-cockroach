provider "cockroach" {
  # Instructions for using the CockroachDB Cloud API
  # https://www.cockroachlabs.com/docs/cockroachcloud/cloud-api.html
  #
  # Instructions for getting an API Key
  # https://www.cockroachlabs.com/docs/cockroachcloud/console-access-management.html#api-access
  #
  # The Terraform provider requires either the COCKROACH_API_KEY or COCKROACH_API_JWT environment variable for performing authentication.
  # export COCKROACH_API_KEY="the API Key value here"
  # export COCKROACH_API_JWT="the JWT value here"
}
