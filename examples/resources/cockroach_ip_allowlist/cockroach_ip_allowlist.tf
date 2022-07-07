 resource "cockroach_allow_list" "cockroach" {
    name = "default-allow-list"
    cidr_ip = "192.168.3.2"
    cidr_mask = 32
    ui = true
    sql = true
    id = cockroach_cluster.cockroach.id
}