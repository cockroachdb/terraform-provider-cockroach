resource "cockroach_cmek" "advanced" {
  id = cockroach_cluster.advanced.id
  regions = [{
    region : "us-central-1"
    key : {
      auth_principal : "arn:aws:iam::account:role/role-name-with-path"
      type : "AWS_KMS"
      uri : "arn:aws:kms:us-west-2:111122223333:key/id-of-kms-key"
    }
  }]
}
