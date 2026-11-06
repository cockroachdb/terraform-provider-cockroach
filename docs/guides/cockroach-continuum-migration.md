# Migrating to Cockroach Continuum

In a Cockroach Continuum organization a cluster has an `edition` rather than a
`plan`. The editions are `STANDARD` and `MISSION_CRITICAL`, and they map onto
the legacy plans like this:

| Legacy plan     | Continuum edition  |
|-----------------|--------------------|
| `STANDARD`      | `STANDARD`         |
| `ADVANCED`      | `MISSION_CRITICAL` |
| `BASIC`         | none               |

Basic clusters have no Continuum equivalent, so an organization with Basic
clusters is currently ineligible for migration. Migration is currently
available to be requested for invoice organizations.  Inquire with your account
representative.

## What the provider does at migration

After the organization is migrated, the API reports an `edition` for every cluster
and no longer reports a `plan`. To keep an existing unmigrated configuration working, the
provider uses a continuum compatibility mode flag when making api requests.  This allows for delayed or piecemeal migration of the terraform config.

While compatibility mode is on, both `plan` and `edition` are populated and a
reference to either resolves. It is enabled automatically, and in common cases
will be disabled automatically once a configuration has been migrated. The
`continuum_compat` provider option turns it off explicitly; only `false` is
meaningful, as there is no way to force compatibility mode on.

* **Clusters that don't set `plan` or `edition`** keep working as they are. The provider cannot tell when `plan` is safe to retire for these, so a warning is issued for each of them. The warning can be cleared either by adding the recommended `edition` or disabling compatibility mode in the provider settings.

* **Clusters that do set `plan`** will eventually need to set `edition` instead.  A warning will be issued for each cluster until this is done.

* **Configurations that read the `plan` field from a data source** should be updated to read the `edition` instead.  Since the provider is unable to detect the reliance on the legacy `plan` field, a warning will be issued for as long as any cluster data source is in use. It clears only when compatibility mode is disabled.

## Converting `plan` to `edition`

Replace the attribute with the edition it maps to:

```diff
 resource "cockroach_cluster" "example" {
   name           = "example"
   cloud_provider = "GCP"
-  plan           = "ADVANCED"
+  edition        = "MISSION_CRITICAL"

   dedicated = {
     storage_gib      = 15
     num_virtual_cpus = 4
   }

   regions = [{
     name       = "us-central1"
     node_count = 3
   }]
 }
```

Swapping a `plan` for the edition it maps to is not a tier change and applies
without replacing the cluster. Swapping it for a different tier is
rejected, as changing a cluster's edition is not supported.

The conversion can be done one resource at a time.

## Converting data source references

For a cluster in a Continuum organization a `cockroach_cluster` data source
reports both an `edition` and the `plan` that edition came from, no matter how
much of the rest of the configuration has been migrated.

Move anything reading the `plan` attribute to `edition`. This could be an output, a module or a conditional.

```terraform
data "cockroach_cluster" "example" {
  id = var.cluster_id
}

output "cluster_edition" {
  value = data.cockroach_cluster.example.edition
}
```

Keep in mind that `plan` and `edition` do not resolve to the same values.  When comparing against the `"ADVANCED"` plan, the equivalent check against edition should be `"MISSION_CRITICAL"`.

## Network visibility

Cockroach Continuum clusters run on private nodes. An Advanced cluster that is
not already on private nodes will be converted (when possible) during the
migration, so afterwards it may report
`dedicated.private_network_visibility = true`. Some older clusters may not be
eligible for this conversion. It is recommended to remove this value from your
configuration altogether to avoid conflicts with the server-driven value.

## Migrating

Setting `continuum_compat = false` on the provider turns off compatibility mode
and will result in an error for each resource that still sets `plan`. Pass it as
a variable override to see everything that is left:

```terraform
variable "continuum_compat" {
  type    = bool
  default = true
}

provider "cockroach" {
  continuum_compat = var.continuum_compat
}
```

```shell
TF_VAR_continuum_compat=false terraform plan
```

Because the report is an error it also blocks `apply`, so leave the variable at
its default until the conversion is finished. The report only covers
`cockroach_cluster` resources. Notably, it cannot see what consumes a data source's
`plan` attribute, so those references have to be found by hand.

Committing `continuum_compat = false` is the last step. It is also what stops a
data source reporting a `plan` and clears the remaining warnings.

## Creating clusters after migration

All new clusters in a Continuum organization require an `edition` rather than a `plan`.
There is no default value.
