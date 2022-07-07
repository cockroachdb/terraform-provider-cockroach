resource "cockroach_cluster" "cockroach" {
    name           = "cockroach-dedicated"
    cloud_provider = "GCP"
    create_spec = {
    dedicated: {
        region_nodes = {
          "us-central1": 1
        }
        hardware = {
          storage_gib = 15
          machine_spec = {
            machine_type = "n1-standard-4"
          }
        }
    }
   }
}
