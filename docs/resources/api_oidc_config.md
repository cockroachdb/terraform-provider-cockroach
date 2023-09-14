---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "cockroach_api_oidc_config Resource - terraform-provider-cockroach"
subcategory: ""
description: |-
  Configuration to allow external OIDC providers to issue tokens for use with CC API.
---

# cockroach_api_oidc_config (Resource)

Configuration to allow external OIDC providers to issue tokens for use with CC API.



<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `audience` (String) The audience that CC API should accept for this API OIDC Configuration.
- `issuer` (String) The issuer of tokens for the API OIDC Configuration. Usually this is a url.
- `jwks` (String) The JSON Web Key Set used to check the signature of the JWTs.

### Optional

- `claim` (String) The JWT claim that should be used as the user identifier. Defaults to the subject.
- `identity_map` (Attributes List) The mapping rules to convert token user identifiers into a new form. (see [below for nested schema](#nestedatt--identity_map))

### Read-Only

- `id` (String) ID of the API OIDC Configuration.

<a id="nestedatt--identity_map"></a>
### Nested Schema for `identity_map`

Required:

- `cc_identity` (String) The username (email or service account id) of the CC user that the token should map to.
- `token_identity` (String) The token value that needs to be mapped.

Optional:

- `is_regex` (Boolean) Indicates that the token_principal field is a regex value.

