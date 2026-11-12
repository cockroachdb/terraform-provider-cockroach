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
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v10/pkg/client"
	mock_client "github.com/cockroachdb/terraform-provider-cockroach/mock"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/stretchr/testify/require"
)

// compatCtxMatcher asserts whether a call asked for the legacy Continuum view.
// The mock Service replaces the whole client, so the request the SDK would
// build is never made; the context holds the decision and is what can be
// checked here.
type compatCtxMatcher struct {
	want bool
}

func (m compatCtxMatcher) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	return ok && continuumCompatFromCtx(ctx) == m.want
}

func (m compatCtxMatcher) String() string {
	if m.want {
		return "a context asking for Continuum compat"
	}
	return "a context not asking for Continuum compat"
}

func compatCtx(want bool) gomock.Matcher {
	return compatCtxMatcher{want: want}
}

// sdkHeaderCtxKey is the key client.ContextWithHeader stores per-call headers
// under. The SDK keeps it unexported, so recover it from the lookup
// ContextWithHeader makes on its parent context.
var sdkHeaderCtxKey = func() any {
	recorder := &ctxKeyRecorder{Context: context.Background()}
	client.ContextWithHeader(recorder, "Probe", "1")
	return recorder.key
}()

type ctxKeyRecorder struct {
	context.Context
	key any
}

func (r *ctxKeyRecorder) Value(key any) any {
	r.key = key
	return nil
}

func continuumCompatFromCtx(ctx context.Context) bool {
	header, _ := ctx.Value(sdkHeaderCtxKey).(http.Header)
	return header.Get(continuumCompatHeader) == "true"
}

// compatAppliedResponse is the response of a legacy read whose cluster really
// had an edition translated away.
func compatAppliedResponse() *http.Response {
	return &http.Response{
		Status: http.StatusText(http.StatusOK),
		Header: http.Header{continuumCompatAppliedHeader: []string{"true"}},
	}
}

// wrappedPattern matches literal text that Terraform may have wrapped across
// lines when it rendered the diagnostic.
func wrappedPattern(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = regexp.QuoteMeta(w)
	}
	return strings.Join(words, `\s+`)
}

// isolateContinuumDetection clears the detection tally, which is package-level
// and so shared by every test in the binary.
func isolateContinuumDetection(t *testing.T) {
	t.Helper()
	resetContinuumDetection()
	t.Cleanup(resetContinuumDetection)
}

func TestContinuumCompatHeaderSentOnRequest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		compat     bool
		wantHeader string
	}{
		{name: "asked", compat: true, wantHeader: "true"},
		{name: "not asked", compat: false, wantHeader: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get(continuumCompatHeader)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{}"))
			}))
			defer srv.Close()

			cfg := client.NewConfiguration("fake")
			cfg.ServerURL = srv.URL
			_, _, err := client.NewService(client.NewClient(cfg)).GetCluster(
				continuumCompatIf(context.Background(), tc.compat), uuid.Nil.String())
			require.NoError(t, err)

			// The server parses the value with strconv.ParseBool, so presence
			// alone would not opt the call in.
			require.Equal(t, tc.wantHeader, got)
		})
	}
}

func TestContinuumCompatApplied(t *testing.T) {
	require.True(t, continuumCompatApplied(compatAppliedResponse()))
	require.False(t, continuumCompatApplied(httpOk))
	require.False(t, continuumCompatApplied(nil))
}

func TestContinuumDataSourceWarningClaimedOnce(t *testing.T) {
	isolateContinuumDetection(t)

	require.True(t, claimContinuumDataSourceWarning())
	require.False(t, claimContinuumDataSourceWarning())

	resetContinuumDetection()
	require.True(t, claimContinuumDataSourceWarning())
}

func TestContinuumCompatSendRules(t *testing.T) {
	withPlan := &CockroachCluster{Plan: types.StringValue(string(client.PLANTYPE_ADVANCED))}
	withoutPlan := &CockroachCluster{Plan: types.StringNull()}

	t.Run("write follows the planned state", func(t *testing.T) {
		p := &provider{}
		require.True(t, p.sendCompatForWrite(withPlan))
		require.False(t, p.sendCompatForWrite(withoutPlan))
		// A create has no prior state to pin a plan from, so nothing is sent
		// and the request has no plan either.
		require.False(t, p.sendCompatForWrite(&CockroachCluster{Plan: types.StringUnknown()}))
	})

	t.Run("read follows prior state", func(t *testing.T) {
		p := &provider{}
		require.True(t, p.sendCompatForRead(withPlan))
		require.False(t, p.sendCompatForRead(withoutPlan))
	})

	t.Run("data source always asks", func(t *testing.T) {
		p := &provider{}
		require.True(t, p.sendCompatForDataSourceRead())
	})

	t.Run("continuum_compat false suppresses every path", func(t *testing.T) {
		p := &provider{continuumCompatOff: true}
		require.False(t, p.sendCompatForWrite(withPlan))
		require.False(t, p.sendCompatForRead(withPlan))
		require.False(t, p.sendCompatForDataSourceRead())
	})
}

func TestContinuumEditionForPlan(t *testing.T) {
	for _, tc := range []struct {
		plan    client.PlanType
		edition client.EditionType
		ok      bool
	}{
		{plan: client.PLANTYPE_ADVANCED, edition: client.EDITIONTYPE_MISSION_CRITICAL, ok: true},
		{plan: client.PLANTYPE_STANDARD, edition: client.EDITIONTYPE_STANDARD, ok: true},
		{plan: client.PLANTYPE_BASIC, ok: false},
	} {
		t.Run(string(tc.plan), func(t *testing.T) {
			edition, ok := continuumEditionForPlan(tc.plan)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.edition, edition)
			}
		})
	}
}

func TestContinuumSynthesizeEdition(t *testing.T) {
	for _, tc := range []struct {
		plan    client.PlanType
		edition string
	}{
		{plan: client.PLANTYPE_ADVANCED, edition: string(client.EDITIONTYPE_MISSION_CRITICAL)},
		{plan: client.PLANTYPE_STANDARD, edition: string(client.EDITIONTYPE_STANDARD)},
		// BASIC has no edition, and a compat-applied response can't report it:
		// the substitution only happens for a cluster that had an edition.
		{plan: client.PLANTYPE_BASIC},
	} {
		t.Run(string(tc.plan), func(t *testing.T) {
			state := &CockroachCluster{Plan: types.StringValue(string(tc.plan))}
			continuumSynthesizeEdition(state)
			if tc.edition == "" {
				require.True(t, state.Edition.IsNull())
				return
			}
			require.Equal(t, tc.edition, state.Edition.ValueString())
		})
	}
}

func TestContinuumKeepPlannedEdition(t *testing.T) {
	advanced := types.StringValue(string(client.PLANTYPE_ADVANCED))
	missionCritical := types.StringValue(string(client.EDITIONTYPE_MISSION_CRITICAL))

	// A compat write reports only the plan, so the planned edition the prior
	// state supplied has to survive into the result.
	state := &CockroachCluster{Plan: advanced, Edition: types.StringNull()}
	continuumKeepPlannedEdition(state, &CockroachCluster{Plan: advanced, Edition: missionCritical})
	require.Equal(t, missionCritical, state.Edition)

	// A legacy organization has no planned edition, so nothing is copied.
	state = &CockroachCluster{Plan: advanced, Edition: types.StringNull()}
	continuumKeepPlannedEdition(state, &CockroachCluster{Plan: advanced, Edition: types.StringNull()})
	require.True(t, state.Edition.IsNull())

	// A response already on the edition axis is left alone.
	state = &CockroachCluster{Plan: types.StringNull(), Edition: missionCritical}
	continuumKeepPlannedEdition(state, &CockroachCluster{Plan: advanced, Edition: types.StringNull()})
	require.Equal(t, missionCritical, state.Edition)
}

func TestContinuumCompatDetail(t *testing.T) {
	resourceDetail := continuumResourceCompatDetail(client.PLANTYPE_ADVANCED)
	require.Contains(t, resourceDetail, `plan = "ADVANCED"`)
	require.Contains(t, resourceDetail, `edition = "MISSION_CRITICAL"`)

	require.Contains(t, continuumDataSourceCompatDetail, "plan attribute")
	// Nothing in the configuration clears this one, so the detail has to say
	// what does.
	require.Contains(t, continuumDataSourceCompatDetail, "continuum_compat = false")

	// A silent cluster has two ways out, and the state knows which edition to
	// recommend.
	silentDetail := continuumSilentResourceCompatDetail(client.EDITIONTYPE_MISSION_CRITICAL)
	require.Contains(t, silentDetail, `edition = "MISSION_CRITICAL"`)
	require.Contains(t, silentDetail, "continuum_compat = false")

	// A plan with no equivalent edition gets no swap to recommend.
	basicDetail := continuumResourceCompatDetail(client.PLANTYPE_BASIC)
	require.NotContains(t, basicDetail, `edition = "`)
	require.Contains(t, basicDetail, "should set edition instead of plan")
}

func TestContinuumPlanReplacementAdvice(t *testing.T) {
	advanced := continuumPlanReplacementAdvice(types.StringValue(string(client.PLANTYPE_ADVANCED)))
	require.Contains(t, advanced, `plan = "ADVANCED" with edition = "MISSION_CRITICAL"`)

	// Basic has no edition to move to, so pointing at the mapping would be
	// advice the practitioner can't follow.
	basic := continuumPlanReplacementAdvice(types.StringValue(string(client.PLANTYPE_BASIC)))
	require.NotContains(t, basic, "MISSION_CRITICAL")
	require.Contains(t, basic, "not eligible for Continuum")

	// A plan that isn't known yet gets the whole mapping.
	unknown := continuumPlanReplacementAdvice(types.StringUnknown())
	require.Contains(t, unknown, "ADVANCED becomes MISSION_CRITICAL")
}

// legacyPlanClusterConfig is a serverless cluster configured the pre-Continuum
// way, on the plan axis.
func legacyPlanClusterConfig(resourceName, clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "%s" {
    name           = "%s"
    cloud_provider = "GCP"
    plan           = "STANDARD"
    serverless = {
        usage_limits = {
            provisioned_virtual_cpus = 2
        }
    }
    regions = [{ name = "`+testRegion+`" }]
}
`, resourceName, clusterName)
}

// silentClusterConfig sets neither plan nor edition, which is what the majority
// of configurations look like.
func silentClusterConfig(resourceName, clusterName string) string {
	return silentClusterConfigWithCPUs(resourceName, clusterName, 2)
}

func silentClusterConfigWithCPUs(resourceName, clusterName string, cpus int) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "%s" {
    name           = "%s"
    cloud_provider = "GCP"
    serverless = {
        usage_limits = {
            provisioned_virtual_cpus = %d
        }
    }
    regions = [{ name = "`+testRegion+`" }]
}
`, resourceName, clusterName, cpus)
}

func legacyPlanCluster(clusterName, id string, plan client.PlanType) client.Cluster {
	p := plan
	return client.Cluster{
		Id:               id,
		Name:             clusterName,
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            client.CLUSTERSTATETYPE_CREATED,
		Plan:             &p,
		Regions:          []client.Region{{Name: testRegion}},
		Config: client.ClusterConfig{
			Serverless: &client.ServerlessClusterConfig{
				UpgradeType: client.UPGRADETYPETYPE_AUTOMATIC,
				UsageLimits: &client.UsageLimits{ProvisionedVirtualCpus: ptr(int64(2))},
				RoutingId:   "routing-id",
			},
		},
	}
}

// TestIntegrationContinuumCompatSentForLegacyConfig verifies that a
// configuration still on the plan axis asks for the legacy view on every call,
// so a migrated organization keeps reporting the plan the configuration names.
func TestIntegrationContinuumCompatSentForLegacyConfig(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-legacy-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := legacyPlanCluster(clusterName, uuid.Nil.String(), client.PLANTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(true), cluster.Id).
		Return(&cluster, compatAppliedResponse(), nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: legacyPlanClusterConfig("test", clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "plan", "STANDARD"),
					resource.TestCheckNoResourceAttr("cockroach_cluster.test", "edition"),
				),
			},
		},
	})
}

// TestIntegrationContinuumCompatNotSentForEditionConfig verifies that a
// configuration on the edition axis never asks for the legacy view. It also
// pins that every cluster resource is validated during a normal run: if
// detection under-reported, the refresh would ask for the legacy view and the
// expectation below would fail.
func TestIntegrationContinuumCompatNotSentForEditionConfig(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-edition-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := editionCluster(clusterName, client.EDITIONTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(false), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(false), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: editionClusterConfig(clusterName, client.EDITIONTYPE_STANDARD),
				Check:  resource.TestCheckResourceAttr(serverlessResourceName, "edition", "STANDARD"),
			},
		},
	})
}

// TestIntegrationContinuumCompatMixedConfig verifies that one resource still on
// the plan axis holds every cluster in the configuration on the legacy view.
// The silent resource keeps its plan rather than moving to an edition, which is
// what keeps the two axes in agreement while any resource still needs the
// legacy one.
func TestIntegrationContinuumCompatMixedConfig(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	legacyName := fmt.Sprintf("%s-compat-mixed-a-%s", tfTestPrefix, GenerateRandomString(2))
	silentName := fmt.Sprintf("%s-compat-mixed-b-%s", tfTestPrefix, GenerateRandomString(2))
	legacyID := "00000000-0000-0000-0000-000000000001"
	silentID := "00000000-0000-0000-0000-000000000002"
	legacy := legacyPlanCluster(legacyName, legacyID, client.PLANTYPE_STANDARD)
	silent := legacyPlanCluster(silentName, silentID, client.PLANTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).Return(&legacy, httpOk, nil)
	// The silent resource sets no plan, so its own create doesn't ask for the
	// legacy view; its refresh does, because a sibling still needs it.
	s.EXPECT().CreateCluster(compatCtx(false), gomock.Any()).Return(&silent, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(true), legacyID).Return(&legacy, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(gomock.Any(), silentID).Return(&silent, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), gomock.Any()).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), gomock.Any()).Return(&legacy, httpOk, nil).Times(2)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: legacyPlanClusterConfig("legacy", legacyName) +
					silentClusterConfig("silent", silentName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.legacy", "plan", "STANDARD"),
					resource.TestCheckResourceAttr("cockroach_cluster.silent", "plan", "STANDARD"),
				),
			},
		},
	})
}

// TestIntegrationContinuumReadinessReport verifies that continuum_compat =
// false reports every resource still on the plan axis, and stays quiet about
// one that isn't. Reporting all of them in a single run is what makes the flag
// a rehearsal for the migration rather than a one-at-a-time hunt.
func TestIntegrationContinuumReadinessReport(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	config := `
provider "cockroach" {
    continuum_compat = false
}
` + legacyPlanClusterConfig("first", "compat-report-first") +
		legacyPlanClusterConfig("second", "compat-report-second") +
		silentClusterConfig("third", "compat-report-third")

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:   config,
				PlanOnly: true,
				ExpectError: regexp.MustCompile(`(?s)cockroach_cluster.first.*cockroach_cluster.second.*` +
					wrappedPattern(`Replace plan = "STANDARD" with edition = "STANDARD".`)),
			},
		},
	})
}

// TestIntegrationContinuumReadinessReportNetworkVisibility verifies that the
// report also covers the attribute that has to change alongside the plan. An
// Advanced cluster becomes Mission Critical, which is always on private IP
// addresses, so a pinned public visibility would fail after the plan is
// converted and not before.
func TestIntegrationContinuumReadinessReportNetworkVisibility(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	config := fmt.Sprintf(`
provider "cockroach" {
    continuum_compat = false
}

resource "cockroach_cluster" "advanced" {
    name           = "%s"
    cloud_provider = "GCP"
    plan           = "ADVANCED"
    dedicated = {
        machine_type               = "n2-standard-2"
        storage_gib                = 15
        private_network_visibility = false
    }
    regions = [{
        name       = "`+testRegion+`"
        node_count = 1
    }]
}
`, fmt.Sprintf("%s-compat-report-netvis", tfTestPrefix))

	// Terraform versions differ in which of the two diagnostics they print
	// first, so accept either order.
	netVisibility := wrappedPattern("Remove private_network_visibility.")
	planAxis := wrappedPattern(`Replace plan = "ADVANCED" with edition = "MISSION_CRITICAL".`)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:   config,
				PlanOnly: true,
				ExpectError: regexp.MustCompile(`(?s)` +
					netVisibility + `.*` + planAxis + `|` + planAxis + `.*` + netVisibility),
			},
		},
	})
}

// editionClusterConfigNamed is editionClusterConfig with a choice of resource
// label, so a configuration can hold more than one.
func editionClusterConfigNamed(
	resourceName, clusterName string, edition client.EditionType,
) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "%s" {
    name           = "%s"
    cloud_provider = "GCP"
    edition        = "%s"
    serverless = {
        usage_limits = {
            provisioned_virtual_cpus = 2
        }
    }
    regions = [{ name = "`+testRegion+`" }]
}
`, resourceName, clusterName, edition)
}

// TestIntegrationContinuumEquivalentSwap verifies that moving one cluster off
// its legacy plan onto the edition that plan already maps to is allowed. The
// tier doesn't change, so the edition immutability check must not refuse it.
//
// A second resource stays on plan throughout, which is what a configuration
// migrated a resource at a time looks like and what keeps the swapping
// resource reading on the legacy view while its own configuration has already
// moved. That combination is the only one the immutability check sees.
func TestIntegrationContinuumEquivalentSwap(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	swapName := fmt.Sprintf("%s-compat-swap-%s", tfTestPrefix, GenerateRandomString(2))
	holdoutName := fmt.Sprintf("%s-compat-holdout-%s", tfTestPrefix, GenerateRandomString(2))
	swapID := "00000000-0000-0000-0000-000000000001"
	holdoutID := "00000000-0000-0000-0000-000000000002"
	legacy := legacyPlanCluster(swapName, swapID, client.PLANTYPE_STANDARD)
	migrated := editionCluster(swapName, client.EDITIONTYPE_STANDARD)
	migrated.Id = swapID
	holdout := legacyPlanCluster(holdoutName, holdoutID, client.PLANTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).DoAndReturn(
		func(_ context.Context, req *client.CreateClusterRequest) (*client.Cluster, *http.Response, error) {
			if req.Name == holdoutName {
				return &holdout, httpOk, nil
			}
			return &legacy, httpOk, nil
		}).Times(2)
	s.EXPECT().GetCluster(gomock.Any(), holdoutID).
		Return(&holdout, compatAppliedResponse(), nil).AnyTimes()
	// The holdout keeps the legacy view on, so the swapping cluster still reads
	// a plan; its own write and read-back go without it.
	s.EXPECT().GetCluster(compatCtx(true), swapID).
		Return(&legacy, compatAppliedResponse(), nil).AnyTimes()
	s.EXPECT().GetCluster(compatCtx(false), swapID).Return(&migrated, httpOk, nil).AnyTimes()
	s.EXPECT().UpdateCluster(compatCtx(false), swapID, gomock.Any()).
		Return(&migrated, httpOk, nil)
	s.EXPECT().GetBackupConfiguration(gomock.Any(), gomock.Any()).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), gomock.Any()).Return(&migrated, httpOk, nil).Times(2)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: legacyPlanClusterConfig("holdout", holdoutName) +
					legacyPlanClusterConfig("swap", swapName),
				Check: resource.TestCheckResourceAttr("cockroach_cluster.swap", "plan", "STANDARD"),
			},
			{
				Config: legacyPlanClusterConfig("holdout", holdoutName) +
					editionClusterConfigNamed("swap", swapName, client.EDITIONTYPE_STANDARD),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.swap", "edition", "STANDARD"),
					resource.TestCheckNoResourceAttr("cockroach_cluster.swap", "plan"),
					resource.TestCheckResourceAttr("cockroach_cluster.holdout", "plan", "STANDARD"),
				),
			},
		},
	})
}

func advancedClusterConfigNamed(resourceName, clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "%s" {
    name           = "%s"
    cloud_provider = "GCP"
    plan           = "ADVANCED"
    dedicated = {
        machine_type = "n2-standard-2"
        storage_gib  = 15
    }
    regions = [{
        name       = "`+testRegion+`"
        node_count = 1
    }]
}
`, resourceName, clusterName)
}

func missionCriticalClusterConfigNamed(resourceName, clusterName string) string {
	return fmt.Sprintf(`
resource "cockroach_cluster" "%s" {
    name           = "%s"
    cloud_provider = "GCP"
    edition        = "MISSION_CRITICAL"
    dedicated = {
        machine_type = "n2-standard-2"
        storage_gib  = 15
    }
    regions = [{
        name       = "`+testRegion+`"
        node_count = 1
    }]
}
`, resourceName, clusterName)
}

func legacyAdvancedCluster(clusterName, id string) client.Cluster {
	plan := client.PLANTYPE_ADVANCED
	// The legacy view reports visibility from the security add-on, which this
	// cluster doesn't have.
	visibility := client.NETWORKVISIBILITYTYPE_PUBLIC
	return client.Cluster{
		Id:                id,
		Name:              clusterName,
		CockroachVersion:  latestClusterPatchVersion,
		CloudProvider:     "GCP",
		State:             client.CLUSTERSTATETYPE_CREATED,
		Plan:              &plan,
		NetworkVisibility: &visibility,
		Regions:           []client.Region{{Name: testRegion, NodeCount: 1}},
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "n2-standard-2",
				NumVirtualCpus: 2,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
	}
}

// TestIntegrationContinuumEquivalentSwapDedicated is the dedicated half of
// TestIntegrationContinuumEquivalentSwap, where network visibility can differ
// between the two views: the legacy view reports it from the security add-on,
// which an Advanced cluster need not have.
// private_network_visibility is optional+computed with UseStateForUnknown, so
// the planned value has to be released or the apply fails as an inconsistent
// result. Released rather than set to true, so a cluster that stays public
// reports what it has.
func TestIntegrationContinuumEquivalentSwapDedicated(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}

	for _, tc := range []struct {
		name       string
		visibility client.NetworkVisibilityType
		want       string
	}{
		{name: "converted to private nodes", visibility: client.NETWORKVISIBILITYTYPE_PRIVATE, want: "true"},
		{name: "stays on public nodes", visibility: client.NETWORKVISIBILITYTYPE_PUBLIC, want: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateContinuumDetection(t)

			clusterName := fmt.Sprintf("%s-compat-swap-mc-%s", tfTestPrefix, GenerateRandomString(2))
			clusterID := uuid.Nil.String()
			legacy := legacyAdvancedCluster(clusterName, clusterID)
			migrated := editionCluster(clusterName, client.EDITIONTYPE_MISSION_CRITICAL)
			migrated.Id = clusterID
			visibility := tc.visibility
			migrated.NetworkVisibility = &visibility

			ctrl := gomock.NewController(t)
			s := mock_client.NewMockService(ctrl)
			defer HookGlobal(&NewService, func(c *client.Client) client.Service {
				return s
			})()

			s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).Return(&legacy, httpOk, nil)
			s.EXPECT().GetCluster(compatCtx(true), clusterID).
				Return(&legacy, compatAppliedResponse(), nil).AnyTimes()
			s.EXPECT().GetCluster(compatCtx(false), clusterID).Return(&migrated, httpOk, nil).AnyTimes()
			s.EXPECT().UpdateCluster(compatCtx(false), clusterID, gomock.Any()).
				Return(&migrated, httpOk, nil)
			s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
				Return(initialBackupConfig, httpOk, nil).AnyTimes()
			s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(&migrated, httpOk, nil)

			resource.Test(t, resource.TestCase{
				IsUnitTest:               true,
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: advancedClusterConfigNamed("swap", clusterName),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr("cockroach_cluster.swap", "plan", "ADVANCED"),
							resource.TestCheckResourceAttr("cockroach_cluster.swap",
								"dedicated.private_network_visibility", "false"),
						),
					},
					{
						Config: missionCriticalClusterConfigNamed("swap", clusterName),
						Check: resource.ComposeTestCheckFunc(
							resource.TestCheckResourceAttr("cockroach_cluster.swap", "edition", "MISSION_CRITICAL"),
							resource.TestCheckNoResourceAttr("cockroach_cluster.swap", "plan"),
							resource.TestCheckResourceAttr("cockroach_cluster.swap",
								"dedicated.private_network_visibility", tc.want),
						),
					},
				},
			})
		})
	}
}

// TestIntegrationContinuumSilentConfigUpdate verifies that a configuration
// setting neither plan nor edition still asks for the legacy view when it
// updates a cluster whose state holds a plan. Both attributes are
// optional+computed, so the planned state keeps the plan a compat read put
// there even though nothing in the configuration asked for one, and the update
// body includes it. Without the header the API rejects that body.
func TestIntegrationContinuumSilentConfigUpdate(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-silent-update-%s", tfTestPrefix, GenerateRandomString(2))
	clusterID := uuid.Nil.String()
	before := legacyPlanCluster(clusterName, clusterID, client.PLANTYPE_STANDARD)
	after := legacyPlanCluster(clusterName, clusterID, client.PLANTYPE_STANDARD)
	after.Config.Serverless.UsageLimits = &client.UsageLimits{ProvisionedVirtualCpus: ptr(int64(4))}
	current := &before

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).Return(&before, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(true), clusterID).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			return current, compatAppliedResponse(), nil
		}).AnyTimes()
	s.EXPECT().UpdateCluster(gomock.Any(), clusterID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, req *client.UpdateClusterSpecification,
		) (*client.Cluster, *http.Response, error) {
			// Asserted here rather than through a context matcher: an
			// unmatched expectation fails from the provider's own goroutine,
			// which hangs the test binary instead of reporting.
			require.NotNil(t, req.Plan, "the planned plan should reach the request body")
			require.True(t, continuumCompatFromCtx(ctx),
				"an update that sets a plan has to ask the API to accept it")
			current = &after
			return current, compatAppliedResponse(), nil
		})
	s.EXPECT().GetBackupConfiguration(gomock.Any(), clusterID).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), clusterID).Return(&after, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: legacyPlanClusterConfig("test", clusterName),
				Check:  resource.TestCheckResourceAttr("cockroach_cluster.test", "plan", "STANDARD"),
			},
			{
				Config: silentClusterConfigWithCPUs("test", clusterName, 4),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cockroach_cluster.test", "plan", "STANDARD"),
					resource.TestCheckResourceAttr("cockroach_cluster.test", "edition", "STANDARD"),
				),
			},
		},
	})
}

// TestIntegrationContinuumSilentCreateOnLegacyOrg keeps the edition from
// becoming a plan-time requirement. The provider can't tell a Continuum
// organization from a legacy one, and a configuration setting neither axis has
// to keep working on a legacy one.
func TestIntegrationContinuumSilentCreateOnLegacyOrg(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-legacy-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := legacyPlanCluster(clusterName, uuid.Nil.String(), client.PLANTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(false), gomock.Any()).Return(&cluster, httpOk, nil)
	// The create asks for no compat, but the plan the API reported puts the
	// state on the legacy axis, so every refresh after it asks.
	s.EXPECT().GetCluster(compatCtx(false), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetCluster(compatCtx(true), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: silentClusterConfig("test", clusterName),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessResourceName, "plan", "STANDARD"),
					resource.TestCheckNoResourceAttr(serverlessResourceName, "edition"),
				),
			},
		},
	})
}

// TestIntegrationContinuumImportSendsNoCompat verifies that an import reads on
// the edition axis. There is no prior state to hold it on the legacy one.
func TestIntegrationContinuumImportSendsNoCompat(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-import-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := editionCluster(clusterName, client.EDITIONTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(false), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: editionClusterConfig(clusterName, client.EDITIONTYPE_STANDARD),
			},
			{
				ResourceName:      serverlessResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestIntegrationContinuumDataSourceReportsBothAxes verifies that a data source
// on a Continuum organization reports the plan the legacy view returned and the
// edition that plan came from, so a reference to either resolves. The
// configuration is only a data source, so nothing in it holds the legacy view:
// a data source asks for it unconditionally.
func TestIntegrationContinuumDataSourceReportsBothAxes(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-ds-both-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := legacyPlanCluster(clusterName, uuid.Nil.String(), client.PLANTYPE_ADVANCED)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().GetCluster(compatCtx(true), cluster.Id).
		Return(&cluster, compatAppliedResponse(), nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "cockroach_cluster" "test" {
    id = "%s"
}
`, cluster.Id),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessDataSourceName, "plan", "ADVANCED"),
					resource.TestCheckResourceAttr(serverlessDataSourceName, "edition", "MISSION_CRITICAL"),
				),
			},
		},
	})
}

// TestIntegrationContinuumDataSourceLegacyOrgKeepsOnePlan verifies that a data
// source on an organization that has not migrated reports a plan and no
// edition. The request asks for the legacy view either way, so the response
// header is what separates a translated edition from a cluster that never had
// one.
func TestIntegrationContinuumDataSourceLegacyOrgKeepsOnePlan(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-ds-legacy-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := legacyPlanCluster(clusterName, uuid.Nil.String(), client.PLANTYPE_ADVANCED)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().GetCluster(compatCtx(true), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "cockroach_cluster" "test" {
    id = "%s"
}
`, cluster.Id),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessDataSourceName, "plan", "ADVANCED"),
					resource.TestCheckNoResourceAttr(serverlessDataSourceName, "edition"),
				),
			},
		},
	})
}

// TestIntegrationContinuumDataSourceRetires verifies that continuum_compat =
// false is what drops a plan from a data source. It is the last step of a
// migration, taken once the references to that attribute have been moved.
func TestIntegrationContinuumDataSourceRetires(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	cluster := editionCluster("compat-ds-retire", client.EDITIONTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().GetCluster(compatCtx(false), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "cockroach" {
    continuum_compat = false
}

data "cockroach_cluster" "test" {
    id = "%s"
}
`, cluster.Id),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(serverlessDataSourceName, "edition", "STANDARD"),
					resource.TestCheckNoResourceAttr(serverlessDataSourceName, "plan"),
				),
			},
		},
	})
}

// TestIntegrationContinuumDataSourceHeldOnLegacyView verifies that a `.plan`
// reference keeps reporting a plan while some cluster resource in the same
// configuration still sets one.
func TestIntegrationContinuumDataSourceHeldOnLegacyView(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-ds-held-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := legacyPlanCluster(clusterName, uuid.Nil.String(), client.PLANTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(true), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(true), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: legacyPlanClusterConfig("test", clusterName) + `
data "cockroach_cluster" "test" {
    id = cockroach_cluster.test.id
}
`,
				Check: resource.TestCheckResourceAttr(serverlessDataSourceName, "plan", "STANDARD"),
			},
		},
	})
}

// TestIntegrationContinuumReadinessReportCleanConfig verifies that
// continuum_compat = false is silent on a configuration already on the edition
// axis, and that the provider reads on that axis. This is the state the
// readiness report is asking practitioners to reach.
func TestIntegrationContinuumReadinessReportCleanConfig(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-clean-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := editionCluster(clusterName, client.EDITIONTYPE_STANDARD)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(compatCtx(false), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(compatCtx(false), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "cockroach" {
    continuum_compat = false
}
` + editionClusterConfig(clusterName, client.EDITIONTYPE_STANDARD),
				Check: resource.TestCheckResourceAttr(serverlessResourceName, "edition", "STANDARD"),
			},
		},
	})
}

// TestIntegrationContinuumNetworkVisibilityDiagnostic verifies the targeted
// message for a cluster whose network visibility flipped when the provider
// stopped asking for the legacy view. A legacy read reports visibility from
// the security add-on, so an Advanced cluster without it reads public and
// reads private once it reports its edition. The configuration here sets
// neither plan nor edition, which is the only shape that reaches this check:
// an explicit MISSION_CRITICAL with public visibility is refused earlier.
func TestIntegrationContinuumNetworkVisibilityDiagnostic(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-compat-netvis-%s", tfTestPrefix, GenerateRandomString(2))
	legacy := dedicatedVisibilityCluster(clusterName, false)
	migrated := dedicatedVisibilityCluster(clusterName, true)

	// Flipped between steps to stand in for the organization migrating between
	// two applies of an unchanged configuration.
	orgMigrated := false

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&legacy, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), legacy.Id).DoAndReturn(
		func(_ context.Context, _ string) (*client.Cluster, *http.Response, error) {
			if orgMigrated {
				return &migrated, httpOk, nil
			}
			return &legacy, httpOk, nil
		}).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), legacy.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), legacy.Id).Return(&migrated, httpOk, nil)

	config := fmt.Sprintf(`
resource "cockroach_cluster" "test" {
    name           = "%s"
    cloud_provider = "GCP"
    dedicated = {
        machine_type               = "n2-standard-2"
        storage_gib                = 15
        private_network_visibility = false
    }
    regions = [{
        name       = "`+testRegion+`"
        node_count = 1
    }]
}
`, clusterName)

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.TestCheckResourceAttr(
					serverlessResourceName, "dedicated.private_network_visibility", "false"),
			},
			{
				PreConfig:   func() { orgMigrated = true },
				Config:      config,
				ExpectError: regexp.MustCompile("reports its edition rather than a legacy plan"),
			},
		},
	})
}

// TestIntegrationMissionCriticalPublicNodesFromState verifies that a Mission
// Critical cluster the API reports on public nodes may say so in its
// configuration. The provider refuses private_network_visibility = false on its
// own, since it sends network visibility only when private and the value would
// not survive a round trip, but an edition-axis read showing public nodes is
// evidence the refusal would be wrong.
func TestIntegrationMissionCriticalPublicNodesFromState(t *testing.T) {
	if os.Getenv(CockroachAPIKey) == "" {
		os.Setenv(CockroachAPIKey, "fake")
	}
	isolateContinuumDetection(t)

	clusterName := fmt.Sprintf("%s-public-mc-%s", tfTestPrefix, GenerateRandomString(2))
	cluster := dedicatedVisibilityCluster(clusterName, true)
	cluster.NetworkVisibility = ptr(client.NETWORKVISIBILITYTYPE_PUBLIC)

	ctrl := gomock.NewController(t)
	s := mock_client.NewMockService(ctrl)
	defer HookGlobal(&NewService, func(c *client.Client) client.Service {
		return s
	})()

	s.EXPECT().CreateCluster(gomock.Any(), gomock.Any()).Return(&cluster, httpOk, nil)
	s.EXPECT().GetCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil).AnyTimes()
	s.EXPECT().GetBackupConfiguration(gomock.Any(), cluster.Id).
		Return(initialBackupConfig, httpOk, nil).AnyTimes()
	s.EXPECT().DeleteCluster(gomock.Any(), cluster.Id).Return(&cluster, httpOk, nil)

	config := func(visibility string) string {
		return fmt.Sprintf(`
resource "cockroach_cluster" "test" {
    name           = "%s"
    cloud_provider = "GCP"
    edition        = "MISSION_CRITICAL"
    dedicated = {
        machine_type = "n2-standard-2"
        storage_gib  = 15
        %s
    }
    regions = [{
        name       = "`+testRegion+`"
        node_count = 1
    }]
}
`, clusterName, visibility)
	}

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true,
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(""),
				Check: resource.TestCheckResourceAttr(
					serverlessResourceName, "dedicated.private_network_visibility", "false"),
			},
			{
				Config: config("private_network_visibility = false"),
				Check: resource.TestCheckResourceAttr(
					serverlessResourceName, "dedicated.private_network_visibility", "false"),
			},
		},
	})
}

// dedicatedVisibilityCluster is the same dedicated cluster before and after its
// organization migrates: on the legacy view it reports an Advanced plan and
// public visibility, and on the edition view Mission Critical and private.
func dedicatedVisibilityCluster(clusterName string, migrated bool) client.Cluster {
	cluster := client.Cluster{
		Id:               uuid.Nil.String(),
		Name:             clusterName,
		CockroachVersion: latestClusterPatchVersion,
		CloudProvider:    "GCP",
		State:            client.CLUSTERSTATETYPE_CREATED,
		Regions:          []client.Region{{Name: testRegion, NodeCount: 1}},
		Config: client.ClusterConfig{
			Dedicated: &client.DedicatedHardwareConfig{
				MachineType:    "n2-standard-2",
				NumVirtualCpus: 2,
				StorageGib:     15,
				MemoryGib:      8,
			},
		},
	}
	if migrated {
		cluster.Edition = ptr(client.EDITIONTYPE_MISSION_CRITICAL)
		cluster.NetworkVisibility = ptr(client.NETWORKVISIBILITYTYPE_PRIVATE)
		return cluster
	}
	cluster.Plan = ptr(client.PLANTYPE_ADVANCED)
	cluster.NetworkVisibility = ptr(client.NETWORKVISIBILITYTYPE_PUBLIC)
	return cluster
}
