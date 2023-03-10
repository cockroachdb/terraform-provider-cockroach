variable "cluster_name" {
  type     = string
  nullable = false
}

variable "cloud_provider" {
  type     = string
  nullable = false
  default  = "GCP"
}

variable "cloud_provider_regions" {
  type     = list(string)
  nullable = false
  default  = ["us-east1"]
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
  default  = "n1-standard-2"
}

terraform {
  required_providers {
    cockroach = {
      source = "cockroachdb/cockroach"
    }
  }
}
provider "cockroach" {
  # export COCKROACH_API_KEY with the cockroach cloud API Key
}

resource "cockroach_cluster" "example" {
  name              = var.cluster_name
  cloud_provider    = var.cloud_provider
  dedicated = {
    storage_gib  = var.storage_gib
    machine_type = var.machine_type
  }
  regions = [
    for r in var.cloud_provider_regions : {
      name       = r,
      node_count = var.cluster_node_count
    }
  ]
}

resource "cockroach_client_ca_cert" "example" {
  id = cockroach_cluster.example.id
  x509_pem_cert = "-----BEGIN CERTIFICATE-----\nMIIDmDCCAoCgAwIBAgIUL1h31paxTeFQkqm+BYdJaHoM81MwDQYJKoZIhvcNAQEL\nBQAwUjENMAsGA1UEAwwEdGVzdDELMAkGA1UEBhMCVVMxEzARBgNVBAgMCldhc2hp\nbmd0b24xEDAOBgNVBAcMB1NlYXR0bGUxDTALBgNVBAoMBHRlc3QwHhcNMjMwMjEw\nMDAxMDEzWhcNNDgwMjA0MDAxMDEzWjBSMQ0wCwYDVQQDDAR0ZXN0MQswCQYDVQQG\nEwJVUzETMBEGA1UECAwKV2FzaGluZ3RvbjEQMA4GA1UEBwwHU2VhdHRsZTENMAsG\nA1UECgwEdGVzdDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALs/XgOi\nuw2LBz3Yze07G1cNRKF8tjwI8AFjhKtrDCxBZkGPsecpn5MqHMKC+HlQHL3sVoiM\nNtIzOOV83cAODFqAecep1mtKq32LcynOUEp+Fv5ZoIt+tE13QzGl1g80t4s+IZ4V\nAKWjzDxe9LLiLsOBbU1/yg0ovVxIPXHYdOaM2HcaCpNd4f1W++9cwzFimNximq8v\n6vgDKWGOEHOVWlUbC8dcqGGQJwCTedGw7N9itkEdnMcB/KMnNxPvcbCuPfzqAMVu\nHRGAnoz4R8F+a/A1vyENVSSrax9m6xaXVtsEz9n6YdAu79KlaPdy3urDMeRBGyRP\npD/CS1yixF/wV+kCAwEAAaNmMGQwHQYDVR0OBBYEFIfK+PrY03qGYNm1QiTrUXyE\nr8w0MB8GA1UdIwQYMBaAFIfK+PrY03qGYNm1QiTrUXyEr8w0MA4GA1UdDwEB/wQE\nAwIBBjASBgNVHRMBAf8ECDAGAQH/AgEBMA0GCSqGSIb3DQEBCwUAA4IBAQBGTOyj\nuvXghvL43GWln/HrgqBCmNEkMM8xP9aubsb296sTyy4Yx6+fCDqjWLPkwg8+/4l6\nJOiUzdsVoL6skUWl6cTTI5DqQaaIfhHb0xTavDFK+fYLcPh3peif5R8DDcqTI4OU\nqQAC2bOE7SGkBQjKgdxkL42f8FpSeULTT2k4/ggdMFWhJXBvWivvTaVtEPtOabkM\nva+1dDx2QXJZhVxiUFRfZ2Vrior3Nj0AWKun5cAR/w9W3UqKDtxCcio83nUzlakO\nC0PPVuJUhaT9wIM1r6DR27VBTgQVW279Qr8AmxRZqAxG+leJUEgmvQnEkP0wORwj\nNNLEtNwKBek9MCug\n-----END CERTIFICATE-----"
}