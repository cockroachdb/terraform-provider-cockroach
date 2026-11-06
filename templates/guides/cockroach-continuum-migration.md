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
clusters is currently ineligible for migration.

## Who is eligible

Migration to Continuum is currently available to be requested for invoice organizations.  Inquire with your account representative.

## What the provider does at migration

After the organization is migrated, the API reports an `edition` for every cluster
and no longer reports a `plan`. To keep an existing unmigrated configuration working, the
provider uses a continuum compatibility mode flag when making api requests.  This allows for delayed or piecemeal migration of the terraform config.

* **Clusters that set neither `plan` nor `edition`** keep working as they are. Both attributes are `optional+computed`, and the provider fills in the `edition` that compatibility mode translated away, so the state holds both and a reference to either keeps resolving. Nothing in such a configuration tells the provider when the legacy view is no longer needed, so a warning is issued for each of these clusters naming the edition to set. Setting it is optional — turning compatibility mode off on the provider clears the warning too.

* **Clusters that do set `plan`** will eventually need to set `edition` instead.  A warning will be issued for each cluster until this is done.

* **Configurations that read the `plan` field from a data source** should be updated to read the `edition` instead.  Since we are unable to detect the reliance on the legacy `plan` field, cluster data sources will backfill so that both `plan` and `edition` are populated.  A warning will be issued if there are cluster data sources in use until the config is migrated and the `continuum_compat` provider option is set explicitly to `false`.

## The `continuum_compat` provider option

Compatibility mode needs no configuration. Left unset:

* A `cockroach_cluster` resource uses compatibility mode for as long as its
  state holds a `plan`. Converting the configuration to `edition` clears the
  `plan` on the next apply, and from then on that resource reads on the edition
  axis. A configuration that sets neither attribute keeps both in state
  indefinitely and its reads stay in compatibility mode, which is what the
  per-cluster warning is about.
* A `cockroach_cluster` data source always uses compatibility mode. The API answers with a
  `plan` and no `edition`, so for a Continuum cluster the provider fills the
  `edition` back in from the plan it maps to, and the data source reports both.
  A cluster in an organization that has not migrated reports only a `plan`.

Setting `continuum_compat = false` turns it off everywhere. Only `false` is
meaningful; there is no way to force compatibility mode on. Use it first as a
readiness preview, then as the last step of the migration.

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
without replacing the cluster. Swapping it for a different tier is still
rejected, as changing a cluster's edition is not supported.

The conversion can be done one resource at a time.

A cluster that sets neither attribute is converted the same way: add the
`edition` the warning names. That resource then reads on the edition axis and
stops warning, whatever the rest of the configuration does.

## Converting data source references

For a cluster in a Continuum organization a `cockroach_cluster` data source
reports both an `edition` and the `plan` that edition came from, so a reference
to either resolves. It keeps doing that no matter what the rest of the
configuration does, because the data source cannot see what reads its `plan`
attribute and so cannot tell when the last reference is gone.

Move anything reading that attribute — an output, a module argument, a
conditional — to `edition`:

```terraform
data "cockroach_cluster" "example" {
  id = var.cluster_id
}

output "cluster_edition" {
  value = data.cockroach_cluster.example.edition
}
```

The two attributes do not resolve to the same values, so a comparison against
`"ADVANCED"` becomes a comparison against `"MISSION_CRITICAL"`.

Once the references are moved and no resource sets `plan`, set
`continuum_compat = false` on the provider. That is what stops the data source
reporting a `plan` and clears the warning.

## Network visibility

Cockroach Continuum clusters run on private nodes. An Advanced cluster that is
not already on private nodes is converted during the migration, so afterwards it
reports `dedicated.private_network_visibility = true`.

A configuration that leaves the attribute unset picks up the new value on the
run that replaces `plan` with `edition`, where the plan reports it as known
after apply. A configuration that sets it to `false` reports an error, since
Mission Critical clusters always use private IP addresses. Remove that line — it
can be removed before the migration, and the readiness preview below reports it
alongside the resources that still set `plan`.

## Previewing readiness

Setting `continuum_compat = false` on the provider turns off compatibility mode
everywhere, and reports an error for each resource that still sets `plan`. One
plan run names all of them:

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

Two limits worth knowing:

* The report covers `cockroach_cluster` resources. It cannot see what consumes
  a data source's `plan` attribute, so references to that attribute have to be
  found by hand.
* Because the report is an error, `continuum_compat = false` blocks `apply`
  while any resource still sets `plan`. Pass it as a variable override for the
  preview rather than committing it part-way through a migration.

Committing `continuum_compat = false` is the last step of the migration and not
only a preview: it is also what drops `plan` from a data source and clears both
warnings.

## Creating clusters after migration

A new cluster in a Continuum organization needs an `edition`. The provider does
not choose one: an edition sets a rate card that cannot be changed after the
cluster is created. A configuration that sets neither `plan` nor `edition`
creates a cluster in a legacy organization but is rejected in a Continuum one,
with an error against the `edition` attribute.
