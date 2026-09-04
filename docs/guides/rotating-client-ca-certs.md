# Rotating Client CA Certs

Client CA certs are managed by setting `x509_pem_cert` on the
`cockroach_client_ca_cert` resource. This value is a PEM bundle, so it can
contain more than one CA certificate. The cluster trusts client certificates
signed by any CA in the bundle, which makes it possible to rotate to a new CA
with no downtime by trusting both the old and new CAs during the transition.

Replacing the cert in a single step (swapping the old cert for the new one)
would momentarily leave the cluster trusting only the new CA, and it would
reject any client still presenting a certificate signed by the old CA. To avoid
this, rotate in three steps: trust both CAs, migrate clients, then drop the old
CA.

The examples below assume the old and new certs are provided as variables:

```terraform
variable "old_client_certificate" {
  type        = string
  description = "The current (old) X509 CA certificate in PEM format."
}

variable "new_client_certificate" {
  type        = string
  description = "The new X509 CA certificate in PEM format."
}
```

## Step 1: Trust both the old and new CA certs

Concatenate the existing (old) cert and the new cert into a single PEM bundle and
set it as `x509_pem_cert`:

```terraform
resource "cockroach_client_ca_cert" "prod" {
  id            = cockroach_cluster.prod.id
  x509_pem_cert = join("\n", [var.old_client_certificate, var.new_client_certificate])
}
```

Apply the change:

```shell
terraform apply
```

After this applies, the cluster trusts client certificates signed by either CA.
Clients still presenting a certificate signed by the old CA continue to connect.

## Step 2: Roll out the new cert to clients

Update your clients (application connection configs, drivers, and so on) to
present certificates signed by the new CA. Because the cluster trusts both CAs
during this window, you can migrate clients gradually without disruption. Do not
proceed to the next step until every client presents a certificate signed by the
new CA.

## Step 3: Drop the old CA cert

Once no client relies on the old CA, remove it from the bundle so the cluster
trusts only the new CA:

```terraform
resource "cockroach_client_ca_cert" "prod" {
  id            = cockroach_cluster.prod.id
  x509_pem_cert = var.new_client_certificate
}
```

Apply the change:

```shell
terraform apply
```

The cluster now trusts only the new CA, and the rotation is complete.
