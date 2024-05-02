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
  default  = "CockroachCloudMetricExportRole"
}

variable "iam_policy_name" {
  type     = string
  nullable = false
  default  = "ExampleCockroachCloudMetricExportPolicy"
}

variable "log_group_name" {
  type     = string
  nullable = false
  default  = "example"
}

variable "datadog_site" {
  type     = string
  nullable = false
  default  = "US1"
}

variable "datadog_api_key" {
  type      = string
  nullable  = false
  sensitive = true
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
  name              = var.log_group_name
  retention_in_days = 0
}

# Cross-account AWS IAM role in your AWS account.
resource "aws_iam_role" "example" {
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

resource "aws_iam_policy" "example" {
  name        = var.iam_policy_name
  description = "An example metric export policy"
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
          "arn:aws:logs:*:${var.aws_account_id}:log-group:${var.log_group_name}:*",
          "arn:aws:logs:*:${var.aws_account_id}:log-group:${var.log_group_name}:log-stream:*"
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "example" {
  role       = aws_iam_role.example.name
  policy_arn = aws_iam_policy.example.arn
}

resource "cockroach_metric_export_cloudwatch_config" "example" {
  id             = cockroach_cluster.example.id
  role_arn       = aws_iam_role.example.arn
  log_group_name = var.log_group_name
  target_region  = var.aws_region
}

resource "cockroach_metric_export_datadog_config" "example" {
  id      = cockroach_cluster.example.id
  site    = var.datadog_site
  api_key = var.datadog_api_key
}

resource "cockroach_metric_export_prometheus_config" "example" {
  id = cockroach_cluster.example.id
}