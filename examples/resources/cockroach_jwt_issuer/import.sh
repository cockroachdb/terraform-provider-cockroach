# JWT Issuer ID can be found by running a GET against the Cockroach Cloud API to
# list all existing JWT issuers.
# https://www.cockroachlabs.com/docs/api/cloud/v1#get-/api/v1/jwt-issuers
# format: <jwt issuer id>
terraform import cockroach_jwt_issuer.my_issuer 1f69fdd2-600a-4cfc-a9ba-16995df0d77d
