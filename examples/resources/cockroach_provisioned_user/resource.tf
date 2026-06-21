resource "cockroach_provisioned_user" "alice" {
  user_name    = "alice@example.com"
  display_name = "Alice Example"
  active       = true

  emails = [
    {
      value   = "alice@example.com"
      primary = true
      type    = "work"
    },
  ]

  name = {
    given_name  = "Alice"
    family_name = "Example"
  }
}

# Grant the new user a role. SCIM groups are not managed by this resource;
# use cockroach_user_role_grant for CockroachDB Cloud native roles.
resource "cockroach_user_role_grant" "alice_org_member" {
  user_id = cockroach_provisioned_user.alice.id
  role = {
    role_name     = "ORG_MEMBER"
    resource_type = "ORGANIZATION"
    resource_id   = ""
  }
}
