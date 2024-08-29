package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v2/pkg/client"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	datasource_schema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/stretchr/testify/require"
)

// tfTestPrefix is the test prefix to use when creating resources in acceptance
// tests.  This makes them easier to find when resources are left dangling after
// failed cleanup.
const tfTestPrefix = "tftest"

func addConfigureProviderErr(diagnostics *diag.Diagnostics) {
	diagnostics.AddError(
		"Provider not configured",
		"The provider hasn't been configured before apply, likely because it depends on an unknown value from another resource. This leads to weird stuff happening, so we'd prefer if you didn't do that. Thanks!",
	)
}

// HookGlobal sets `*ptr = val` and returns a closure for restoring `*ptr` to
// its original value. A runtime panic will occur if `val` is not assignable to
// `*ptr`.
func HookGlobal[T any](ptr *T, val T) func() {
	orig := *ptr
	*ptr = val
	return func() { *ptr = orig }
}

// CheckSchemaAttributesMatch is a test utility that can be used to make sure a resource's schema stays in sync with
// its datasource counterpart. It compares attribute names and topology, but not properties.
func CheckSchemaAttributesMatch(t *testing.T, rAttributes map[string]resource_schema.Attribute, dAttributes map[string]datasource_schema.Attribute) {
	for name, rAttr := range rAttributes {
		dAttr, ok := dAttributes[name]
		require.True(t, ok)
		if rNA, ok := rAttr.(resource_schema.NestedAttribute); ok {
			dNA, ok := dAttr.(datasource_schema.NestedAttribute)
			require.True(t, ok)

			// Resource and datasource schemas implement the same base type,
			// but that base type is /internal, so we need to go through some
			// faff to get the raw attribute map.
			rLen := len(rNA.GetNestedObject().GetAttributes())
			require.Equal(t, rLen, len(dNA.GetNestedObject().GetAttributes()))
			rAttrs := make(map[string]resource_schema.Attribute, rLen)
			dAttrs := make(map[string]datasource_schema.Attribute, rLen)
			for name, attr := range rNA.GetNestedObject().GetAttributes() {
				rAttrs[name] = attr
			}
			for name, attr := range dNA.GetNestedObject().GetAttributes() {
				dAttrs[name] = attr
			}
			CheckSchemaAttributesMatch(t, rAttrs, dAttrs)
		}
	}
}

func formatAPIErrorMessage(err error) string {
	apiErr := client.Error{}
	if ok := errors.As(err, &apiErr); ok {
		if status, ok := apiErr.Model().(client.Status); ok {
			return status.GetMessage()
		}
		// If the error doesn't have all the fields we expect in a Status,
		// it'll be unmarshalled into a map instead.
		if model, ok := apiErr.Model().(map[string]interface{}); ok {
			if message, ok := model["message"]; ok {
				return message.(string)
			}
		}
	}
	return err.Error()
}

const uuidRegexString = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"

var uuidRegex = regexp.MustCompile(uuidRegexString)

// uuidValidator is an array of string validators containing just the one
// specific uuid validator. Its specified in this array format for convenience
// because all current and expected future uses won't combine this with other
// validators and it allows using it like so:
//
//   "some_id": schema.StringAttribute{
//       Description:   "the description.",
//       Optional:      true,
//       Validators:    uuidValidator,
//    },
var uuidValidator = []validator.String{stringvalidator.RegexMatches(
	uuidRegex,
	"must match UUID format",
)}

// retryGetRequests implements the retryable-http CheckRetry type.
func retryGetRequestsOnly(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if resp != nil && resp.Request != nil && resp.Request.Method != http.MethodGet {
		// We don't want to blindly retry anything that isn't a GET method
		// because it's possible that a different method type mutated data on
		// the server even if the response wasn't successful. Application code
		// should handle any retries where appropriate.
		return false, nil
	}

	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

// leveledTFLogger implements the retryablehttp.LeveledLogger interface by adapting tflog methods.
type leveledTFLogger struct {
	baseCtx context.Context
}

func (l *leveledTFLogger) llArgsToTFLogArgs(keysAndValues []interface{}) map[string]interface{} {
	if argCount := len(keysAndValues); argCount%2 != 0 {
		tflog.Warn(l.baseCtx, fmt.Sprintf("unexpected number of log arguments: %d", argCount))
		return map[string]interface{}{}
	}
	additionalFields := make(map[string]interface{}, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		additionalFields[keysAndValues[i].(string)] = keysAndValues[i+1]
	}
	return additionalFields
}

func (l *leveledTFLogger) Error(msg string, keysAndValues ...interface{}) {
	tflog.Error(l.baseCtx, msg, l.llArgsToTFLogArgs(keysAndValues))
}
func (l *leveledTFLogger) Info(msg string, keysAndValues ...interface{}) {
	tflog.Info(l.baseCtx, msg, l.llArgsToTFLogArgs(keysAndValues))
}
func (l *leveledTFLogger) Debug(msg string, keysAndValues ...interface{}) {
	tflog.Debug(l.baseCtx, msg, l.llArgsToTFLogArgs(keysAndValues))
}
func (l *leveledTFLogger) Warn(msg string, keysAndValues ...interface{}) {
	tflog.Warn(l.baseCtx, msg, l.llArgsToTFLogArgs(keysAndValues))
}

var _ retryablehttp.LeveledLogger = &leveledTFLogger{}

// formatEnumMarkdownList takes a list of allowed enum values and formats them as a Markdown list.
func formatEnumMarkdownList[T ~string](allowedValues []T) (mdList string) {
	for _, val := range allowedValues {
		mdList += "\n  * " + string(val)
	}
	return mdList
}

type Knowable interface {
	IsUnknown() bool
	IsNull() bool
}

// IsKnown is a shortcut that checks in a value is neither null nor unknown.
func IsKnown[T Knowable](t T) bool {
	return !t.IsUnknown() && !t.IsNull()
}

func ptr[T any](in T) *T {
	return &in
}

// traceAPICall is a helper for debugging which api calls are happening when to
// make it easier to determine for understanding what the provider framework is
// doing and for determining which calls will need to be mocked in our tests.
// Currently it relies on being manually called at each api call site which is
// unfortunate.
func traceAPICall(endpoint string) {
	val, exists := os.LookupEnv("TRACE_API_CALLS")
	if exists && val == "1" {
		pc, _, _, _ := runtime.Caller(1)
		fmt.Printf("CC API Call: %s (%s)\n", endpoint, runtime.FuncForPC(pc).Name())
	}
}
