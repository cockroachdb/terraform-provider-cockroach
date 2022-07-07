variable cluster_id {
    type = string
}

resource "cockroach_cluster" "cockroach" {
    id = "cluster-dedicated"
    name           = "cluster-dedicated"
    cloud_provider = "AWS"
    wait_for_cluster_ready = true
    update_spec = {
      dedicated = {
        region_nodes = {
          "ap-south-1" = 3
        }
        hardware = {
          storage_gib = 25
          machine_spec = {
            machine_type = "m5.large"
          }
        }
      }
    }
}

# resource "cockroach_cluster" "cockroach" {
#     name = var.cluster_id
#     cloud_provider = "AWS"
#     update_spec = {
#         serverless = {
#             spend_limit = 2
#         }
#     }
# }