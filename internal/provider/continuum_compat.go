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
	"net/http"
	"sync"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	// continuumCompatHeader opts a request into the pre-Continuum view of a
	// cluster: the API reports the plan an edition came from and omits the
	// edition, translates a plan on create, and checks a plan against the
	// cluster's edition on update.
	continuumCompatHeader = "Cc-Continuum-Compat"

	// continuumCompatAppliedHeader is set on a response whose cluster actually
	// had an edition translated away. It is the only way to tell a Continuum
	// cluster from a legacy one on a compat read, since both report a plan.
	continuumCompatAppliedHeader = "Cc-Continuum-Compat-Applied"
)

// continuumCompatIf keeps the per-call decision at the call site, so the
// condition reads next to the request it governs.
func continuumCompatIf(ctx context.Context, compat bool) context.Context {
	if !compat {
		return ctx
	}
	// The server parses the value with strconv.ParseBool, so presence alone is
	// not enough.
	return client.ContextWithHeader(ctx, continuumCompatHeader, "true")
}

// continuumCompatApplied reports whether the response translated an edition
// into a plan, which means the cluster is in a Continuum organization.
func continuumCompatApplied(resp *http.Response) bool {
	return resp != nil && resp.Header.Get(continuumCompatAppliedHeader) == "true"
}

// The three compat warnings carry distinct summaries because Terraform
// consolidates warnings by summary, printing the first and collapsing the rest
// into a count. A shared summary hid two of the three details, and each one
// asks for a different change.
const (
	continuumCompatPlanSummary       = "Cluster sets plan on a Continuum organization"
	continuumCompatSilentSummary     = "Cluster sets neither plan nor edition on a Continuum organization"
	continuumCompatDataSourceSummary = "Data source reports a plan on a Continuum organization"
)

// continuumResourceCompatDetail instructs a configuration that sets plan on a
// Continuum organization. Reported from ModifyPlan rather than Read: Read can't
// see the configuration, so it would warn about every cluster in a partly
// converted configuration, including the ones with nothing to fix.
func continuumResourceCompatDetail(plan client.PlanType) string {
	if edition, ok := continuumEditionForPlan(plan); ok {
		return fmt.Sprintf("A Continuum cluster has an edition rather than a plan, so this cluster "+
			"should set edition = %q instead of plan = %q. Compatibility mode is active for this cluster "+
			"until the configuration is converted.", edition, plan)
	}
	return "A Continuum cluster has an edition rather than a plan, so this cluster should set edition " +
		"instead of plan. Compatibility mode is active for this cluster until the configuration is " +
		"converted."
}

// continuumDataSourceCompatDetail is aimed at whatever reads a data source's
// plan attribute. That reference is the one that breaks when the provider stops
// reporting a plan. Reported once for the whole configuration rather than once
// per data source: a data source can't see what reads its plan, so there is
// nothing to narrow the warning down to.
const continuumDataSourceCompatDetail = "This configuration uses cockroach_cluster data sources. A " +
	"data source cannot see what reads its plan attribute, so the provider cannot tell when the last " +
	"reference is gone and keeps reporting one for all of them. Move anything reading plan to " +
	"edition, which does not report the same values: ADVANCED becomes MISSION_CRITICAL and STANDARD " +
	"stays STANDARD. Then set continuum_compat = false on the provider, which stops the data sources " +
	"reporting a plan and clears this warning."

// continuumSilentResourceCompatDetail is aimed at a cluster whose configuration
// sets neither axis. Nothing in that configuration tells the provider when the
// compat mode is no longer needed, so it states the edition the cluster already
// reports and the two ways out.
func continuumSilentResourceCompatDetail(edition client.EditionType) string {
	return fmt.Sprintf("This cluster sets neither plan nor edition, so the provider cannot tell when "+
		"to stop using compatibility mode for it. Its state holds both attributes and a reference to "+
		"either resolves. Once the migration is complete, either set edition = %q on this resource or "+
		"set continuum_compat = false on the provider.", edition)
}

// continuumPlanReplacementAdvice names the edition a configuration should move
// its plan to. A plan that isn't known yet gets the whole mapping.
func continuumPlanReplacementAdvice(configPlan types.String) string {
	if IsKnown(configPlan) {
		plan := client.PlanType(configPlan.ValueString())
		if edition, ok := continuumEditionForPlan(plan); ok {
			return fmt.Sprintf("Replace plan = %q with edition = %q.", plan, edition)
		}
		if plan == client.PLANTYPE_BASIC {
			return "Basic has no equivalent edition, and an organization with Basic clusters is not " +
				"eligible for Continuum, so this cluster has to leave the Basic plan before its " +
				"organization can migrate."
		}
	}
	return "Replace plan with the equivalent edition: ADVANCED becomes MISSION_CRITICAL and " +
		"STANDARD stays STANDARD."
}

// continuumEditionForPlan maps a legacy plan to the edition it became on
// Continuum. BASIC has no edition, so it reports false.
func continuumEditionForPlan(plan client.PlanType) (client.EditionType, bool) {
	switch plan {
	case client.PLANTYPE_ADVANCED:
		return client.EDITIONTYPE_MISSION_CRITICAL, true
	case client.PLANTYPE_STANDARD:
		return client.EDITIONTYPE_STANDARD, true
	default:
		return "", false
	}
}

// continuumPlanForEdition maps an edition back to the legacy plan it came from.
func continuumPlanForEdition(edition client.EditionType) (client.PlanType, bool) {
	switch edition {
	case client.EDITIONTYPE_MISSION_CRITICAL:
		return client.PLANTYPE_ADVANCED, true
	case client.EDITIONTYPE_STANDARD:
		return client.PLANTYPE_STANDARD, true
	default:
		return "", false
	}
}

var continuumDetection struct {
	mu sync.Mutex
	// dataSourceWarned records that the one data source warning was reported.
	dataSourceWarned bool
}

// claimContinuumDataSourceWarning reports whether this read should emit the
// data source warning. One warning covers the configuration, so the first read
// to ask for it takes it.
func claimContinuumDataSourceWarning() bool {
	continuumDetection.mu.Lock()
	defer continuumDetection.mu.Unlock()
	if continuumDetection.dataSourceWarned {
		return false
	}
	continuumDetection.dataSourceWarned = true
	return true
}

// sendCompatForWrite reports whether a create or update should ask the API to
// accept a legacy plan. Keyed on the planned state because that is what fills
// in spec.plan on the request. Keying on the configuration instead would let a
// silent configuration send a plan it never asked for, pinned from a compat
// read, with no header to make the API accept it.
func (p *provider) sendCompatForWrite(plan *CockroachCluster) bool {
	return !p.continuumCompatOff && IsKnown(plan.Plan)
}

// sendCompatForRead reports whether a refresh should be a compat read.
// Keyed on state rather than on the configuration: Read has no configuration,
// and the axis the state already expresses is the one the response has to
// agree with.
func (p *provider) sendCompatForRead(state *CockroachCluster) bool {
	return !p.continuumCompatOff && !state.Plan.IsNull()
}

// synthesizePlanForDataSource reports whether a data source read should fill in
// the legacy plan. Deliberately not tied to detection: a data source can't see
// what reads its plan attribute, so dropping the attribute under it would break
// that reference with nothing to catch it. Setting continuum_compat = false is
// what releases it.
func (p *provider) synthesizePlanForDataSource() bool {
	return !p.continuumCompatOff
}

// continuumSynthesizeEdition fills in the edition a compat read translated
// away, so the state holds both axes and a reference to either resolves.
// Only correct for a response that actually had an edition substituted; a
// legacy cluster's plan does not stand for one.
func continuumSynthesizeEdition(state *CockroachCluster) {
	if edition, ok := continuumEditionForPlan(client.PlanType(state.Plan.ValueString())); ok {
		state.Edition = types.StringValue(string(edition))
	}
}

// continuumSynthesizePlan fills in the plan an edition came from, so a data
// source holds both axes and a reference to either resolves. It reports whether
// it filled one in, which doubles as the signal that the cluster is in a
// Continuum organization: only those report an edition. Deriving the plan here
// rather than making a compat read keeps the rest of the response
// describing the cluster as it actually is.
func continuumSynthesizePlan(state *CockroachCluster) bool {
	plan, ok := continuumPlanForEdition(client.EditionType(state.Edition.ValueString()))
	if !ok {
		return false
	}
	state.Plan = types.StringValue(string(plan))
	return true
}

// continuumKeepPlannedEdition copies the edition Terraform planned into the
// new state when a compat write translated it away. A compat response reports
// only the plan, so without this the result disagrees with the planned edition
// the prior state supplied and the apply fails. A legacy organization has no
// planned edition to keep, so nothing happens.
func continuumKeepPlannedEdition(state, plan *CockroachCluster) {
	if state.Edition.IsNull() && !state.Plan.IsNull() && plan != nil && IsKnown(plan.Edition) {
		state.Edition = plan.Edition
	}
}

func resetContinuumDetection() {
	continuumDetection.mu.Lock()
	defer continuumDetection.mu.Unlock()
	continuumDetection.dataSourceWarned = false
}
