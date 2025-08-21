# Import an egress private endpoint using cluster_id:endpoint_id format
terraform import cockroach_egress_private_endpoint.app_service 1f69fdd2-600a-4cfc-a9ba-16995df0d77d:87654321-1234-5678-9abc-def012345678

# Import multiple egress private endpoints
terraform import cockroach_egress_private_endpoint.msk_sasl_scram 1f69fdd2-600a-4cfc-a9ba-16995df0d77d:12345678-abcd-efgh-ijkl-mnopqrstuvwx
terraform import cockroach_egress_private_endpoint.msk_iam 1f69fdd2-600a-4cfc-a9ba-16995df0d77d:abcdef12-3456-7890-abcd-ef1234567890
terraform import cockroach_egress_private_endpoint.msk_tls 1f69fdd2-600a-4cfc-a9ba-16995df0d77d:fedcba98-7654-3210-fedc-ba9876543210