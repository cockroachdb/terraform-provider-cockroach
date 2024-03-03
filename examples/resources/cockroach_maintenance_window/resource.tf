variable "cluster_id" {
  type = string
}

variable "offset_duration" {
  type = number
  # 2 days, i.e. window starts at WED 00:00 UTC.
  default = 172800
}

variable "window_duration" {
  type = number
  # 6 hours.
  default = 21600
}

resource "cockroach_maintenance_window" "example" {
  id              = var.cluster_id
  offset_duration = var.offset_duration
  window_duration = var.window_duration
}
