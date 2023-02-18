# Your CockroachDB Organization ID can be found at 
# https://cockroachlabs.cloud/information
variable "org_id" {
  type     = string
  nullable = false
}

# Your AWS Account ID (not the AWS Account ID 
# of your CockroachDB Dedicated cluster).
variable "aws_account_id" {
  type     = string
  nullable = false
}

variable "aws_region" {
  type     = string
  default  = "us-east-1"
  nullable = false
}

variable "cluster_name" {
  type     = string
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

variable "iam_role_name" {
  type     = string
  nullable = false
  default  = "CockroachCloudLogExportRole"
}

variable "iam_policy_name" {
  type     = string
  nullable = false
  default  = "ExampleCockroachCloudLogExportPolicy"
}

variable "log_group_name" {
  type     = string
  nullable = false
  default  = "example"
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
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [{
    name       = var.aws_region,
    node_count = var.cluster_node_count
    }
  ]
}

resource "aws_cloudwatch_log_group" "example" {
  name = var.log_group_name
}

# Cross-account AWS IAM role in your AWS account.
resource "aws_iam_role" "example-role" {
  name = var.iam_role_name

  assume_role_policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Effect" : "Allow",
        "Action" : "sts:AssumeRole",
        "Principal" : {
          "AWS" : cockroach_cluster.example.account_id
        }
      }
    ]
  })
}

resource "aws_iam_policy" "example-policy" {
  name        = var.iam_policy_name
  description = "An example log export policy"
  policy = jsonencode({
    "Version" : "2012-10-17",
    "Statement" : [
      {
        "Action" : [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:DescribeLogGroups",
          "logs:DescribeLogStreams",
          "logs:PutRetentionPolicy",
          "logs:PutLogEvents"
        ],
        "Effect" : "Allow",
        "Resource" : [
          "arn:aws:logs:*:${var.aws_account_id}:log-group:${var.log_group_name}:*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "example-attach" {
  role       = aws_iam_role.example-role.name
  policy_arn = aws_iam_policy.example-policy.arn
}

resource "cockroach_log_export_config" "example" {
  id             = cockroach_cluster.example.id
  auth_principal = aws_iam_role.example-role.arn
  log_name       = var.log_group_name
  type           = "AWS_CLOUDWATCH"
  redact         = true
  region         = var.aws_region
  groups = [
    {
      log_name  = "sql",
      channels  = ["SQL_SCHEMA", "SQL_EXEC"],
      min_level = "WARNING"
    },
    {
      log_name = "devops",
      channels = ["OPS", "HEALTH", "STORAGE"],
      redact   = false
    }
  ]
}
