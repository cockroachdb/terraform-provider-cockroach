# Your Organization ID can be found at https://cockroachlabs.cloud/information
variable "org_id" {
  type     = string
  nullable = false
}

# Required to assign yourself permission to update the key.
variable "iam_user" {
  type     = string
  nullable = false
}

variable "cluster_name" {
  type     = string
  nullable = false
}

variable "aws_region" {
  type     = string
  nullable = false
  default  = "us-west-2"
}

variable "additional_regions" {
  type     = list(string)
  nullable = false
}

variable "cluster_node_count" {
  type     = number
  nullable = false
  default  = 3
}

variable "storage_gib" {
  type     = number
  nullable = false
  default  = 15
}

variable "machine_type" {
  type     = string
  nullable = false
  default  = "m5.large"
}

terraform {
  required_providers {
    cockroach = {
      source = "cockroachdb/cockroach"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}
provider "cockroach" {
  # export COCKROACH_API_KEY with the cockroach cloud API Key
}

provider "aws" {
  # See https://registry.terraform.io/providers/hashicorp/aws/latest/docs
  # for configuration steps.

  # Please don't use a variable for region in production! The AWS provider won't
  # be able to find any resources if this value changes and you'll get
  # into a weird state. Be sure to run `terraform destroy` before changing
  # this value.
  region = var.aws_region
}

resource "cockroach_cluster" "example" {
  name           = var.cluster_name
  cloud_provider = "AWS"
  dedicated = {
    storage_gib                = var.storage_gib
    machine_type               = var.machine_type
    private_network_visibility = true
  }
  regions = [{
    name       = var.aws_region,
    node_count = var.cluster_node_count
    }
  ]
}

resource "aws_iam_role" "example" {
  name = "cmek_test_role"

  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : "sts:AssumeRole",
        "Principal" : {
          "AWS" : cockroach_cluster.example.account_id
        },
        "Condition" : {
          "StringEquals" : {
            "sts:ExternalId" : var.org_id
          }
        }
      }
    ]
  })
}

data "aws_iam_user" "example" {
  user_name = var.iam_user
}

resource "aws_kms_key" "example" {
  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : "kms:*",
        "Principal" : {
          "AWS" : [
            aws_iam_role.example.arn,
            data.aws_iam_user.example.arn
          ]
        },
        "Resource" : "*"
      }
    ]
  })
  multi_region = true
}

resource "cockroach_cmek" "example" {
  id = cockroach_cluster.example.id
  regions = /*concat(*/ [
    {
      region : var.aws_region
      key : {
        auth_principal : aws_iam_role.example.arn
        type : "AWS_KMS"
        uri : aws_kms_key.example.arn
      }
    }
  ] #,
  #
  # Additional regions can be added after CMEK is enabled by updating
  # the `region` attribute and adding their name and node count to
  # `additional_regions`. These regions will be managed separately from
  # the parent cluster, but will otherwise behave the same. Cluster data
  # sources will always show the entire list of regions, regardless of
  # whether they're managed by the cluster or CMEK resource.
  #
  # These should be concatenated with the current region(s).
  #[for r in var.additional_regions : {
  #  region: r,
  #  key: {
  #    auth_principal: aws_iam_role.example.arn
  #    type: "AWS_KMS"
  #    uri: aws_kms_key.example.arn
  #  }
  #}])

  #additional_regions = [for r in var.additional_regions :
  #  {
  #    name = r
  #    node_count = var.cluster_node_count
  #  }
  #]
}
