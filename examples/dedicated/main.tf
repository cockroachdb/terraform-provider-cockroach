terraform {
  required_providers {
    cockroach = {
      source = "registry.terraform.io/hashicorp/cockroach"
    }
  }
}
provider "cockroach" {
# export COCKROACH_API_KEY with the cockroach cloud API Key
}

resource "cockroach_cluster" "cockroach" {
    name           = "cockroach-dedicated"
    cloud_provider = "AWS"
    wait_for_cluster_ready = true
    create_spec = {
    dedicated: {
      region_nodes = {
        "ap-south-1": 1
      }
      hardware = {
        storage_gib = 15
        machine_spec = {
          machine_type = "m5.large"
        }
      }
    }
   }
}
 resource "cockroach_allow_list" "cockroach" {
    name = "default-allow-list"
    cidr_ip = "192.168.3.2"
    cidr_mask = 32
    ui = true
    sql = true
    id = cockroach_cluster.cockroach.id
}

resource "cockroach_sql_user" "cockroach" {
  name = "default-user"
  password = "default@123456"
  id = cockroach_cluster.cockroach.id
}

