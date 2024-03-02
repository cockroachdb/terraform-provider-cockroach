# Cluster and folder level role grants can be imported using:
# <user_id>,<role_name>,<resource_type>,<resource_id>
terraform import cockroach_user_role_grant.admin_grant 1f69fdd2-600a-4cfc-a9ba-16995df0d77d,CLUSTER_ADMIN,CLUSTER,9b9d23fe-3848-40b2-a3c5-d8ccb1c4f831

# Organization level grants can omit the resource_id
terraform import cockroach_user_role_grant.org_level_grant 1f69fdd2-600a-4cfc-a9ba-16995df0d77d,ORG_ADMIN,ORGANIZATION
