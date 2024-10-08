---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "cockroach_api_key Resource - terraform-provider-cockroach"
subcategory: ""
description: |-
  API Keys can be used for programmatic access to the cockroach cloud api. Each key is mapped to a cockroachserviceaccount service_account.
  To access the secret, declare an output value for it and use the terraform output command. i.e. terraform output -raw example_secret
  During API key creation, a sensitive key is created and stored in the terraform state. Always follow best practices https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state when managing sensitive data.
---

# cockroach_api_key (Resource)

API Keys can be used for programmatic access to the cockroach cloud api. Each key is mapped to a [cockroach_service_account](service_account).
		
To access the secret, declare an output value for it and use the terraform output command. i.e. `terraform output -raw example_secret` 

During API key creation, a sensitive key is created and stored in the terraform state. Always follow [best practices](https://developer.hashicorp.com/terraform/tutorials/configuration-language/sensitive-variables#sensitive-values-in-state) when managing sensitive data.

## Example Usage

```terraform
resource "cockroach_api_key" "example" {
  name               = "An example api key"
  service_account_id = cockroach_service_account.example_sa.id
}

output "example_secret" {
  value       = cockroach_api_key.example.secret
  description = "The api key for the example api key"
  sensitive   = true
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `name` (String) Name of the api key.
- `service_account_id` (String)

### Read-Only

- `created_at` (String) Creation time of the api key.
- `id` (String) The ID of this resource.
- `secret` (String, Sensitive)

## Import

Import is supported using the following syntax:

```shell
# Since the secret, is not retreivable after creation, it must be provided
# during import.  The API key ID can be derived from the secret.
# format: terraform import <resource> <api key secret>
terraform import cockroach_api_key.example CCDB1_D4zMI3pZTmk5rGrzYqMhbc_NkcXLI8d81Mtx3djD45iwPfgtnaRv0XCh0Z9047K
```
