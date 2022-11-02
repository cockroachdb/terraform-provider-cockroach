variable "cluster_name" {
  type     = string
  nullable = false
}

variable "sql_user_name" {
  type     = string
  nullable = false
  default  = "maxroach"
}

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

variable "cluster_nodes" {
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
  default  = "n1-standard-2"
}

variable "subnets" {
  type     = map(any)
  nullable = false
  default = {
    "subnet-0" = "10.0.0.0/17"
    "subnet-1" = "10.128.0.0/17"
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
  # see https://registry.terraform.io/providers/hashicorp/aws/latest/docs
  # for configuration steps
  region = var.aws_region
}

resource "cockroach_cluster" "cockroach" {
  name           = var.cluster_name
  cloud_provider = "AWS"
  dedicated = {
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [{
    name = var.aws_region
    node_count = var.cluster_nodes
  }]
}

resource "cockroach_sql_user" "cockroach" {
  name       = var.sql_user_name
  password   = var.sql_user_password
  cluster_id = cockroach_cluster.cockroach.id
}

resource "cockroach_private_endpoint_services" "cockroach" {
  cluster_id = cockroach_cluster.cockroach.id
}

resource "aws_vpc" "cockroach" {
  cidr_block           = var.vpc_cidr_block
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = {
    Name = "cockroach vpc"
  }
}

resource "aws_security_group" "cockroach" {
  name        = "cockroach"
  description = "Allow inbound access to CockroachDB from EC2"
  vpc_id      = aws_vpc.cockroach.id

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

resource "aws_subnet" "cockroach" {
  for_each = var.subnets

  vpc_id     = aws_vpc.cockroach.id
  cidr_block = each.value
  tags = {
    Name = each.key
  }
}

resource "aws_vpc_endpoint" "cockroach" {
  vpc_id              = aws_vpc.cockroach.id
  service_name        = cockroach_private_endpoint_services.cockroach.services[0].aws.service_name
  vpc_endpoint_type   = "Interface"
  security_group_ids  = [aws_security_group.cockroach.id]
  subnet_ids          = [for s in aws_subnet.cockroach : s.id]
  #private_dns_enabled = true
  tags = {
    Name = "cockroach vpc"
  }
}

resource "cockroach_private_endpoint_connection" "cockroach" {
  cluster_id  = cockroach_cluster.cockroach.id
  endpoint_id = aws_vpc_endpoint.cockroach.id
}
