/*
Copyright 2026 The Cockroach Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provider

import (
	"context"
	"fmt"
	"time"

	tftimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var _ validator.String = minTimeoutValidator{}

type minTimeoutValidator struct {
	minimum time.Duration
}

func (v minTimeoutValidator) Description(context.Context) string {
	return fmt.Sprintf("must be at least %s", formatTimeoutDuration(v.minimum))
}

func (v minTimeoutValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v minTimeoutValidator) ValidateString(
	ctx context.Context, req validator.StringRequest, resp *validator.StringResponse,
) {
	value := req.ConfigValue
	if !IsKnown(value) {
		return
	}

	duration, err := time.ParseDuration(value.ValueString())
	if err != nil {
		// The timeouts package's built-in validator reports parse errors.
		return
	}
	if duration >= v.minimum {
		return
	}

	resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(
		req.Path,
		"Invalid Timeout Duration",
		fmt.Sprintf("%q %s.", value.ValueString(), v.Description(ctx)),
	))
}

func formatTimeoutDuration(duration time.Duration) string {
	if duration%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(duration/time.Hour))
	}
	return duration.String()
}

// timeoutsBlockWithMinimums builds a create/update timeouts block that rejects
// values shorter than the operation's default budget. Shortening the timeout
// below the default can only make a legitimate operation fail early, so it is
// treated as a configuration error rather than silently honored.
func timeoutsBlockWithMinimums(
	ctx context.Context, createMin, updateMin time.Duration,
) schema.Block {
	block := tftimeouts.Block(ctx, tftimeouts.Opts{Create: true, Update: true})
	nested, ok := block.(schema.SingleNestedBlock)
	if !ok {
		return block
	}

	for name, minimum := range map[string]time.Duration{"create": createMin, "update": updateMin} {
		attr, ok := nested.Attributes[name].(schema.StringAttribute)
		if !ok {
			continue
		}
		attr.Validators = append(attr.Validators, minTimeoutValidator{minimum: minimum})
		attr.Description = appendMinimumDescription(attr.Description, minimum)
		attr.MarkdownDescription = appendMinimumDescription(attr.MarkdownDescription, minimum)
		nested.Attributes[name] = attr
	}

	return nested
}

// timeoutOperation selects which timeout resolveTimeout reads.
type timeoutOperation int

const (
	timeoutCreate timeoutOperation = iota
	timeoutUpdate
)

func (op timeoutOperation) attrName() string {
	if op == timeoutCreate {
		return "create"
	}
	return "update"
}

// resolveTimeout returns the timeout to use for op and whether the practitioner
// explicitly configured it.
//
// When configured is true, the caller should bound the whole operation with the
// returned value (an end-to-end deadline the practitioner can size to a CI/CD
// wall-clock limit). When configured is false, the value is the legacy default
// and callers apply it per wait phase as before, preserving behavior for configs
// that omit the block. Returning the value alongside the flag keeps the resolve
// and the "is it set?" decision in one place.
func resolveTimeout(
	ctx context.Context, t tftimeouts.Value, op timeoutOperation, def time.Duration,
) (time.Duration, bool, diag.Diagnostics) {
	var (
		timeout time.Duration
		diags   diag.Diagnostics
	)
	if op == timeoutCreate {
		timeout, diags = t.Create(ctx, def)
	} else {
		timeout, diags = t.Update(ctx, def)
	}
	return timeout, timeoutAttrConfigured(t, op.attrName()), diags
}

// timeoutAttrConfigured reports whether the named timeout was explicitly set in
// the practitioner's timeouts block (versus falling back to the default).
func timeoutAttrConfigured(t tftimeouts.Value, name string) bool {
	v, ok := t.Object.Attributes()[name]
	if !ok {
		return false
	}
	return IsKnown(v)
}

func appendMinimumDescription(description string, minimum time.Duration) string {
	sentence := fmt.Sprintf(
		"Must be at least %s. When set, it bounds the entire operation.",
		formatTimeoutDuration(minimum),
	)
	if description == "" {
		return sentence
	}
	return description + " " + sentence
}
