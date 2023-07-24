# Developing `terraform-provider-cockroach`

## Installation

1. In a terminal clone the `terraform-provider-cockroach` repository:

    ~~~ shell
    git clone https://github.com/cockroachdb/terraform-provider-cockroach.git
    ~~~

1. Navigate to the `terraform-provider-cockroach` directory.

    ~~~ shell
    cd terraform-provider-cockroach
    ~~~

1. Build the binary and copy it to your path.

    ~~~ shell
    make install
    ~~~

## Environment Variables

To override the hostname of the cockroach cloud api server used by the
terraform provider

~~~shell
# no trailing slash!
export COCKROACH_SERVER=https://qaserver.com
~~~

## Regenerating Docs

Documentation is generated based on the `Description` and `MarkdownDescription` fields in the schemas.

~~~shell
make generate
~~~

# Contributor Guide

## Suggested Reading

- [Providers](https://developer.hashicorp.com/terraform/language/providers)
- [Provider Framework](https://developer.hashicorp.com/terraform/plugin/framework)
- [Debugging](https://developer.hashicorp.com/terraform/plugin/debugging)
- [Testing](https://developer.hashicorp.com/terraform/plugin/testing)
- [Best practice design principles](https://developer.hashicorp.com/terraform/plugin/best-practices/hashicorp-provider-design-principles)

## Provider Philosophy

In a nutshell, a Terraform provider tells Terraform how to manage external resources and read from data sources.
Terraform users specify their desired state of the world in one or more [config files](https://developer.hashicorp.com/terraform/language/syntax/configuration).
Terraform then uses the appropriate providers to read the actual state of the world and attempts to use CRUD operations
to make the state match the plan.

### Asynchronous Operations

Terraform operations need to be fully blocking. We can't spin up a dedicated cluster and immediately declare the
resource as created, because that would signal to Terraform that it's ok to start creating dependent resources. Usually,
this means API polling. There are some good examples of how to do this in the cluster and CMEK resources.

## Testing

We have two types of required tests for each resource and data source: Integration tests and acceptance tests. Both
typically call into a shared test method where we specify a series of configs and checks for each step, then run them
with the Terraform acceptance testing framework. The difference is that the integration tests use a mocked API client
library, while the acceptance tests are live and create real resources. The integration test wrapper needs to set
expected mock calls before calling that common method.

Integration and unit tests are run on every commit, while acceptance tests are only automatically run during the release
process. Unit tests are great, but rare since TF provider methods don't usually contain complex business logic.

**It's generally a good idea to run acceptance tests affected by your change manually before submitting a PR.** If
you're using GoLand, you can run them individually, setting `TF_ACC=1` and setting `COCKROACH_SERVER` and
`COCKROACH_API_KEY` to appropriate values.

Getting the mock calls right for integration tests can be challenging. You may be surprised at how many read calls are
issued at each stage. A technique that I've found works well is using `.AnyTimes()` and setting break points in the mock
controller's method implementations, then setting `.Times(n)` to the actual number of calls at each stage.

Tests should generally cover all of a resource's CRUD operations, plus `import`. That generally means at least three
test steps. Data sources can either have their own simple test method or, if convenient, be dropped into an existing
resource test.

There are currently no permanent fixtures for acceptance tests to use. Each test needs to create the resources it
depends on. Prefer using serverless clusters in tests when possible, since they're way quicker to create.

## Releasing

There's currently no set release cadence. Whenever a couple features and/or bug fixes are ready, we kick off a release.

Before starting a new release, first make sure `CHANGELOG.md` is up-to-date. Then it's recommended that you run
`COCKROACH_SERVER=<non-production QA server> COCKROACH_API_KEY=<API key for that server> make testacc`.

To start the release process, simply push a new tag. Tags should follow [semver](https://semver.org/) semantics with a
'v' prefix, e.g. "v1.0.0". From there, it's all GitHub workflow magic. It'll run all tests (including acceptance) and,
if they pass, generate and publish platform-specific binaries. The new version should show up on the [Terraform provider
registry](https://registry.terraform.io/providers/cockroachdb/cockroach/latest) shortly after. Sometimes it can take up
to a day or so, but usually it's much quicker.

## FAQs
**Q: What's the difference between the plan and the config in create/update requests?**

A: The config is the raw set of attributes that are explicitly set in the user's config file(s). Terraform takes this
config and uses it to generate a plan, sometimes using something called a plan modifier. It's technically more correct
to use the plan in most cases, but as of this writing, we make fairly light and simple use of plan modifiers, and the
config is generally easier to work with since you only need to check for `Null`, not `Unknown`.

**Q: When and why do we need both a TF resource and data source for the same CC resource?**

A: Sometimes a preexisting CC resource is valuable as a source of data that other resources rely on, but users might not
want to use Terraform to manage it. An obvious example would be a single production cluster environment. Users may want
to manage users and network configuration in Terraform, and simply read the cluster's ID or AWS account ID without
importing it as a resource.

## Questions?

If you're a CRL employee, feel free to reach out to the App Platform team or hop into #_cc-terraform.

If you're an external contributor, first of all, thank you! The best way to contact us is with a GitHub issue. Any and
all feedback is welcome.
