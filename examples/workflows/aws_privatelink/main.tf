variable "cluster_name" {
  type     = string
  nullable = false
}

variable "sql_user_name" {
  type     = string
  nullable = false
  default  = "maxroach"
}

# Remember that even variables marked sensitive will show up
# in the Terraform state file. Always follow best practices
# when managing sensitive info.
# https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state
variable "sql_user_password" {
  type      = string
  nullable  = false
  sensitive = true
}

variable "aws_region" {
  type     = string
  nullable = false
  default  = "us-east-1"
}

variable "cluster_node_count" {
  type     = number
  nullable = false
  default  = 1
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

variable "subnets" {
  type     = map(any)
  nullable = false
  default = {
    "subnet-0" = "10.0.0.0/16"
  }
}

variable "vpc_cidr_block" {
  type     = string
  nullable = false
  default  = "10.0.0.0/16"
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
    name       = var.aws_region
    node_count = var.cluster_node_count
  }]
}

resource "cockroach_sql_user" "example" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.example.id
}

resource "cockroach_private_endpoint_services" "example" {
  cluster_id = cockroach_cluster.example.id
}

resource "aws_vpc" "example" {
  cidr_block           = var.vpc_cidr_block
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = {
    Name = "cockroach vpc"
  }
}

resource "aws_security_group" "example" {
  name        = "cockroach"
  description = "Allow inbound access to CockroachDB from EC2"
  vpc_id      = aws_vpc.example.id

  ingress {
    description = "CockroachDB"
    from_port   = 26257
    to_port     = 26257
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr_block]
  }
  tags = {
    Name = "cockroach vpc"
  }
}

resource "aws_subnet" "example" {
  for_each = var.subnets

  vpc_id     = aws_vpc.example.id
  cidr_block = each.value
  tags = {
    Name = each.key
  }
}

resource "aws_vpc_endpoint" "example" {
  vpc_id             = aws_vpc.example.id
  service_name       = cockroach_private_endpoint_services.example.services_map[var.aws_region].name
  vpc_endpoint_type  = "Interface"
  security_group_ids = [aws_security_group.example.id]
  subnet_ids         = [for s in aws_subnet.example : s.id]

  # This flag can only be set after the connection has been accepted by creating
  # the cockroach_private_endpoint_connection resource.
  #
  # private_dns_enabled = true

  tags = {
    Name = "cockroach vpc"
  }
}

resource "cockroach_private_endpoint_connection" "example" {
  cluster_id  = cockroach_cluster.example.id
  endpoint_id = aws_vpc_endpoint.example.id
}
