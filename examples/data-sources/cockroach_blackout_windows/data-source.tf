# List all blackout windows (upto 100)
data "cockroach_blackout_windows" "all" {
  cluster_id = "123e4567-e89b-12d3-a456-426614174000"
}

# Limit results and sort order 
data "cockroach_blackout_windows" "limited" {
  cluster_id = "123e4567-e89b-12d3-a456-426614174000"
  limit      = 3
  sort_order = "DESC" # default is "ASC"
}

# Pagination: fetch page 1, and then use the token for page 2
data "cockroach_blackout_windows" "page_1" {
  cluster_id = "123e4567-e89b-12d3-a456-426614174000"
  limit      = 2
}

data "cockroach_blackout_windows" "page_2" {
  cluster_id = "123e4567-e89b-12d3-a456-426614174000"
  page       = data.cockroach_blackout_windows.page_1.next_page
}
