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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/stretchr/testify/require"
)

// fakeTB captures errors reported by a recorder so tests can assert on them
// without failing.
type fakeTB struct {
	testing.TB
	errors []string
	logs   []string
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Logf(format string, args ...any) {
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Failed() bool  { return len(f.errors) > 0 }
func (f *fakeTB) Skipped() bool { return false }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func recorderClient(r *recorder) *http.Client {
	return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return r.roundTrip(req, http.DefaultTransport)
	})}
}

func doRequest(t *testing.T, c *http.Client, method, url, body string) (int, string, error) {
	t.Helper()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(data), nil
}

// newFolderServer simulates a resource that is created, polled until ready
// and deleted.
func newFolderServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/folders":
			body, _ := io.ReadAll(r.Body)
			fmt.Fprintf(w, `{"id": "1", "request": %s, "secret": "server-secret"}`, body)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/folders/1":
			if polls.Add(1) <= 3 {
				fmt.Fprint(w, `{"id": "1", "state": "CREATING"}`)
			} else {
				fmt.Fprint(w, `{"id": "1", "state": "CREATED"}`)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/cert":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "-----BEGIN CERTIFICATE-----\n<cert>\n-----END CERTIFICATE-----\n")
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"code": 5, "message": "not found"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func recordFolderTraffic(t *testing.T, path string) (url string, hits *atomic.Int32) {
	t.Helper()
	hits = &atomic.Int32{}
	srv := newFolderServer(t, hits)
	tb := &fakeTB{TB: t}
	r := newRecordingRecorder(tb, path)
	c := recorderClient(r)

	for _, body := range []string{`{"name": "a", "password": "hunter2"}`, `{"name": "b"}`} {
		status, _, err := doRequest(t, c, http.MethodPost, srv.URL+"/api/v1/folders", body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status)
	}
	for range 5 {
		_, _, err := doRequest(t, c, http.MethodGet, srv.URL+"/api/v1/folders/1", "")
		require.NoError(t, err)
	}
	_, _, err := doRequest(t, c, http.MethodGet, srv.URL+"/api/v1/cert", "")
	require.NoError(t, err)
	status, _, err := doRequest(t, c, http.MethodDelete, srv.URL+"/api/v1/folders/1?force=true&a=b", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status)

	r.finish()
	require.Empty(t, tb.errors)
	return srv.URL, hits
}

func TestRecorderRecordAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recordings", "TestFolder.json")
	url, hits := recordFolderTraffic(t, path)
	recordedHits := hits.Load()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	file := string(data)
	require.NotContains(t, file, "hunter2")
	require.NotContains(t, file, "server-secret")
	require.Contains(t, file, redactedValue)
	require.Contains(t, file, "-----BEGIN CERTIFICATE-----")

	tb := &fakeTB{TB: t}
	r, err := newReplayingRecorder(tb, path)
	require.NoError(t, err)

	// The polls are compressed into two interactions: CREATING x3, CREATED x2.
	polls := r.queues[interactionKey(http.MethodGet, "/api/v1/folders/1", "")]
	require.Len(t, polls, 2)
	require.Equal(t, 3, polls[0].ix.Count)
	require.Equal(t, 2, polls[1].ix.Count)

	c := recorderClient(r)

	// Mutations may arrive in a different order, and redacted fields don't
	// need to match.
	_, body, err := doRequest(t, c, http.MethodPost, url+"/api/v1/folders", `{"name": "b"}`)
	require.NoError(t, err)
	require.Contains(t, body, `"name": "b"`)
	_, body, err = doRequest(t, c, http.MethodPost, url+"/api/v1/folders", `{"password": "other", "name": "a"}`)
	require.NoError(t, err)
	require.Contains(t, body, `"name": "a"`)

	var states []string
	for range 7 {
		_, body, err := doRequest(t, c, http.MethodGet, url+"/api/v1/folders/1", "")
		require.NoError(t, err)
		if strings.Contains(body, "CREATING") {
			states = append(states, "CREATING")
		} else {
			states = append(states, "CREATED")
		}
	}
	// Reads past the end of the recording keep getting the last response.
	require.Equal(t, []string{"CREATING", "CREATING", "CREATING", "CREATED", "CREATED", "CREATED", "CREATED"}, states)

	status, body, err := doRequest(t, c, http.MethodGet, url+"/api/v1/cert", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.True(t, strings.HasPrefix(body, "-----BEGIN CERTIFICATE-----\n"))

	// Query parameter order doesn't matter.
	status, _, err = doRequest(t, c, http.MethodDelete, url+"/api/v1/folders/1?a=b&force=true", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, status)

	r.finish()
	require.Empty(t, tb.errors)
	require.Equal(t, recordedHits, hits.Load(), "replay must not call the server")
}

func TestRecorderReplayMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TestFolder.json")
	url, hits := recordFolderTraffic(t, path)
	recordedHits := hits.Load()

	tb := &fakeTB{TB: t}
	r, err := newReplayingRecorder(tb, path)
	require.NoError(t, err)
	c := recorderClient(r)

	_, _, err = doRequest(t, c, http.MethodPost, url+"/api/v1/folders", `{"name": "c"}`)
	require.Error(t, err)
	require.Len(t, tb.errors, 1)
	require.Contains(t, tb.errors[0], `"name":"c"`)
	require.Contains(t, tb.errors[0], "make testrecord")

	_, _, err = doRequest(t, c, http.MethodGet, url+"/api/v1/unknown", "")
	require.Error(t, err)
	require.Len(t, tb.errors, 2)
	require.Contains(t, tb.errors[1], "GET /api/v1/unknown")

	// Neither recorded POST nor the DELETE was replayed.
	tb.errors = nil
	r.finish()
	require.Len(t, tb.errors, 3)
	require.Contains(t, tb.errors[0], "was never requested")
	require.Len(t, tb.logs, 1)
	require.Contains(t, tb.logs[0], "6 recorded reads were never requested")
	require.Equal(t, recordedHits, hits.Load(), "replay must not call the server")
}

func TestRecorderNormalizesIdentifiers(t *testing.T) {
	const (
		clusterID = "5f1c2b7e-3d4a-4e8b-9c6d-2a7b8e9f0a1b"
		orgID     = "C0FFEE00-1234-4ABC-8DEF-0123456789AB"
		account   = "crl-test-abc1"
		principal = "Sa@crl-test-abc1.iam.gserviceaccount.com"
	)
	clusterJSON := fmt.Sprintf(`{"id": %q, "organization_id": %q, "parent_id": %q, "account_id": %q, "gcp_auth_principal": %q}`,
		clusterID, orgID, nilUUID, account, principal)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/clusters", "/api/v1/clusters/" + clusterID:
			fmt.Fprint(w, clusterJSON)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer srv.Close()

	// createAndRead creates a cluster and reads it back by the returned ID,
	// like the provider does.
	createAndRead := func(c *http.Client) string {
		_, body, err := doRequest(t, c, http.MethodPost, srv.URL+"/api/v1/clusters", `{"name": "c"}`)
		require.NoError(t, err)
		id := uuidPattern.FindString(body)
		_, body, err = doRequest(t, c, http.MethodGet, srv.URL+"/api/v1/clusters/"+id, "")
		require.NoError(t, err)
		return body
	}

	path := filepath.Join(t.TempDir(), "TestCluster.json")
	tb := &fakeTB{TB: t}
	rec := newRecordingRecorder(tb, path)
	createAndRead(recorderClient(rec))
	rec.finish()
	require.Empty(t, tb.errors)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	file := string(data)
	for _, real := range []string{clusterID, strings.ToLower(orgID), orgID, account} {
		require.NotContains(t, file, real)
	}
	require.Contains(t, file, `"/api/v1/clusters/00000000-0000-4000-8000-000000000001"`)
	require.Contains(t, file, `"organization_id": "00000000-0000-4000-8000-000000000002"`)
	require.Contains(t, file, `"parent_id": "`+nilUUID+`"`)
	require.Contains(t, file, `"account_id": "account-id-1"`)
	require.Contains(t, file, `"gcp_auth_principal": "gcp-auth-principal-1"`)

	rep, err := newReplayingRecorder(tb, path)
	require.NoError(t, err)
	body := createAndRead(recorderClient(rep))
	require.Contains(t, body, "00000000-0000-4000-8000-000000000001")
	rep.finish()
	require.Empty(t, tb.errors)

	require.Equal(t, rec.rec.Seed, newRecordingRecorder(tb, path).rec.Seed, "re-recording must reuse the seed")
}

func TestRecorderRefusesToSaveAPIKey(t *testing.T) {
	t.Setenv(CockroachAPIKey, "CCDB1_leaked")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"note": "CCDB1_leaked"}`)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "TestLeak.json")
	tb := &fakeTB{TB: t}
	r := newRecordingRecorder(tb, path)
	_, _, err := doRequest(t, recorderClient(r), http.MethodGet, srv.URL+"/api/v1/x", "")
	require.NoError(t, err)
	r.finish()

	require.Len(t, tb.errors, 1)
	require.Contains(t, tb.errors[0], "refusing to write")
	_, err = os.Stat(path)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestRecorderTransportNeverCallsAPIInReplayMode(t *testing.T) {
	t.Setenv(recordingModeEnvVar, recordingModeReplay)
	require.Nil(t, currentRecorder())

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	c := &http.Client{Transport: recorderTransport{inner: http.DefaultTransport}}
	_, _, err := doRequest(t, c, http.MethodGet, srv.URL+"/api/v1/clusters", "")
	require.ErrorContains(t, err, "refusing to call")
	require.Zero(t, hits.Load())
}

func TestRecorderSeededRandomStrings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TestRandom.json")
	tb := &fakeTB{TB: t}
	rec := newRecordingRecorder(tb, path)
	recorded := []string{rec.randomString(4), rec.randomString(2)}
	rec.finish()
	require.Empty(t, tb.errors)

	rep, err := newReplayingRecorder(tb, path)
	require.NoError(t, err)
	require.Equal(t, recorded, []string{rep.randomString(4), rep.randomString(2)})
	require.Equal(t, rec.rec.Now, rep.rec.Now)
}

func TestReplayRetryContext(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	attempts := 0
	err := replayRetryContext(ctx, time.Hour, func() *retry.RetryError {
		attempts++
		if attempts < 50 {
			return retry.RetryableError(errors.New("not ready"))
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 50, attempts)
	require.Less(t, time.Since(start), time.Second)

	err = replayRetryContext(ctx, time.Hour, func() *retry.RetryError {
		return retry.NonRetryableError(errors.New("bad request"))
	})
	require.EqualError(t, err, "bad request")

	err = replayRetryContext(ctx, time.Hour, func() *retry.RetryError {
		return retry.RetryableError(errors.New("still creating"))
	})
	require.ErrorContains(t, err, "recording is likely out of date")
	require.ErrorContains(t, err, "still creating")
}
