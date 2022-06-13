# pre-Alpha
**This is still under initial development, suitable only for experimental use.**


# Terraform provider for CockroachDB Cloud

`terraform-provider-cockroach` is the [Terraform provider](https://learn.hashicorp.com/collections/terraform/providers) for the CockroachDB Cloud API [[reference](https://www.cockroachlabs.com/docs/api/cloud/v1), [getting started](https://www.cockroachlabs.com/docs/cockroachcloud/cloud-api)].

- [More information about CockroachDB](https://www.cockroachlabs.com/)
- [More information about Terraform](https://terraform.io) 

## Getting Started

### Installation
1. Clone this repo and then `cd terraform-provider-cockroach`
2. Run `make install` builds the binary and copies it to the default path

### Usage Example
3. Run `cd examples/cluster` and examine the `main.tf` file
4. For authentication, the provider looks for env variable `COCKROACH_API_KEY`
  - See [instructions for getting API Access](https://www.cockroachlabs.com/docs/cockroachcloud/console-access-management#api-access)
  - Run `export COCKROACH_API_KEY=<YOUR_API_KEY>` to set this env variable
5. Run `terraform init` while in that same directory (which contains the `main.tf` file)
6. Run `terraform plan` to see what will be done (without actually doing it)
7. Run `terraform apply` to go ahead and do it
8. (optionally) Run `terraform destroy` to undo these changes
