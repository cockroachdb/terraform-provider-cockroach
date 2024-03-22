variable "client_certificate" {
  type        = string
  description = "The X509 certificate in PEM format."
  // For example in tfvars:
  //
  //client_certificate = <<-EOT
  //-----BEGIN CERTIFICATE-----
  //MIIDgTCCAmigAwIBAgIBADANBgkqhkiG9w0BAQ0FADBaMQswCQYDVQQGEwJ1czEL
  //MAkGA1UECAwCV0ExDjAMBgNVBAoMBXRlc3QxMQ4wDAYDVQQDDAV0ZXN0MTEOMAwG
  //A1UEBwwFdGVzdDExDjAMBgNVBAsMBXRlc3QxMB4XDTIzMDMyMTE5MDczN1oXDTI2
  //MDMyMDE5MDczN1owWjELMAkGA1UEBhMCdXMxCzAJBgNVBAgMAldBMQ4wDAYDVQQK
  //DAV0ZXN0MTEOMAwGA1UEAwwFdGVzdDExDjAMBgNVBAcMBXRlc3QxMQ4wDAYDVQQL
  //...
  //-----END CERTIFICATE-----
  //EOT
}

resource "cockroach_client_ca_cert" "prod" {
  id            = cockroach_cluster.prod.id
  x509_pem_cert = var.client_certificate
}
