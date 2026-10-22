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
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

const (
	recordingVersion = 1
	redactedValue    = "REDACTED"

	// replayMaxRetries bounds retry loops during replay. Without it, a
	// recording that no longer matches the provider's polling would spin
	// until the operation's timeout, which can be hours.
	replayMaxRetries = 10_000
)

// redactedFields are JSON fields whose string values are never written to a
// recording. Matching compares redacted bodies, so replay doesn't depend on
// them either.
var redactedFields = map[string]bool{
	"api_key":             true,
	"azure_client_secret": true,
	"password":            true,
	"secret":              true,
}

// recording is the on-disk format of a golden file.
type recording struct {
	Version int `json:"version"`
	// Seed drives GenerateRandomString, so a replay sends the same resource
	// names that were sent while recording.
	Seed uint64 `json:"seed"`
	// Now is the time returned by testNow.
	Now          time.Time         `json:"now"`
	Vars         map[string]string `json:"vars,omitempty"`
	Interactions []*interaction    `json:"interactions"`
}

// interaction is one recorded request and its response. Count is the number of
// identical consecutive exchanges for the same method, path and query, which
// keeps polling loops compact.
type interaction struct {
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Query        string          `json:"query,omitempty"`
	RequestBody  json.RawMessage `json:"request_body,omitempty"`
	RequestText  string          `json:"request_text,omitempty"`
	Status       int             `json:"status"`
	ContentType  string          `json:"content_type,omitempty"`
	ResponseBody json.RawMessage `json:"response_body,omitempty"`
	ResponseText string          `json:"response_text,omitempty"`
	Count        int             `json:"count"`
}

func (ix *interaction) key() string {
	return interactionKey(ix.Method, ix.Path, ix.Query)
}

func (ix *interaction) sameExchange(o *interaction) bool {
	return ix.key() == o.key() &&
		bytes.Equal(ix.RequestBody, o.RequestBody) &&
		ix.RequestText == o.RequestText &&
		ix.Status == o.Status &&
		ix.ContentType == o.ContentType &&
		bytes.Equal(ix.ResponseBody, o.ResponseBody) &&
		ix.ResponseText == o.ResponseText
}

func interactionKey(method, path, query string) string {
	if query == "" {
		return method + " " + path
	}
	return method + " " + path + "?" + query
}

type replayEntry struct {
	ix        *interaction
	remaining int
}

// recorder records API traffic to a golden file, or replays it from one.
type recorder struct {
	t      testing.TB
	name   string
	path   string
	replay bool

	mu  sync.Mutex
	rec recording
	rng *rand.Rand

	// lastByKey holds the most recent interaction per key while recording.
	lastByKey map[string]*interaction
	// queues holds the interactions per key, in recorded order, while
	// replaying.
	queues map[string][]*replayEntry
}

func newRecordingRecorder(t testing.TB, path string) *recorder {
	var seedBytes [8]byte
	if _, err := cryptorand.Read(seedBytes[:]); err != nil {
		t.Fatalf("generating recording seed: %v", err)
	}
	r := &recorder{
		t:    t,
		name: t.Name(),
		path: path,
		rec: recording{
			Version: recordingVersion,
			Seed:    binary.LittleEndian.Uint64(seedBytes[:]),
			Now:     time.Now().UTC().Truncate(time.Second),
			Vars:    map[string]string{},
		},
		lastByKey: map[string]*interaction{},
	}
	r.rng = rand.New(rand.NewPCG(r.rec.Seed, r.rec.Seed))
	return r
}

func newReplayingRecorder(t testing.TB, path string) (*recorder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := &recorder{
		t:      t,
		name:   t.Name(),
		path:   path,
		replay: true,
		queues: map[string][]*replayEntry{},
	}
	if err := json.Unmarshal(data, &r.rec); err != nil {
		return nil, fmt.Errorf("parsing recording %s: %w", path, err)
	}
	if r.rec.Version != recordingVersion {
		return nil, fmt.Errorf("recording %s has version %d, expected %d; re-record it with: %s",
			path, r.rec.Version, recordingVersion, rerecordCommand(r.name))
	}
	for _, ix := range r.rec.Interactions {
		count := max(ix.Count, 1)
		r.queues[ix.key()] = append(r.queues[ix.key()], &replayEntry{ix: ix, remaining: count})
	}
	r.rng = rand.New(rand.NewPCG(r.rec.Seed, r.rec.Seed))
	return r, nil
}

func (r *recorder) randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	r.mu.Lock()
	defer r.mu.Unlock()
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.rng.IntN(len(letters))]
	}
	return string(b)
}

// roundTrip records the exchange with inner, or serves it from the recording
// without touching the network.
func (r *recorder) roundTrip(req *http.Request, inner http.RoundTripper) (*http.Response, error) {
	var reqBody []byte
	if req.Body != nil {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	if r.replay {
		return r.serve(req, reqBody)
	}

	outReq := req.Clone(req.Context())
	outReq.Body = io.NopCloser(bytes.NewReader(reqBody))
	resp, err := inner.RoundTrip(outReq)
	if err != nil {
		// Transport errors can't be replayed, so they aren't recorded.
		return nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	ix := &interaction{
		Method:      req.Method,
		Path:        req.URL.Path,
		Query:       normalizeQuery(req.URL.RawQuery),
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Count:       1,
	}
	ix.RequestBody, ix.RequestText = encodeBody(reqBody)
	ix.ResponseBody, ix.ResponseText = encodeBody(respBody)
	r.add(ix)
	return resp, nil
}

func (r *recorder) add(ix *interaction) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if last := r.lastByKey[ix.key()]; last != nil && last.sameExchange(ix) {
		last.Count++
		return
	}
	r.rec.Interactions = append(r.rec.Interactions, ix)
	r.lastByKey[ix.key()] = ix
}

func (r *recorder) serve(req *http.Request, reqBody []byte) (*http.Response, error) {
	key := interactionKey(req.Method, req.URL.Path, normalizeQuery(req.URL.RawQuery))

	r.mu.Lock()
	entry, err := r.match(key, req.Method, reqBody)
	r.mu.Unlock()
	if err != nil {
		r.t.Errorf("%v", err)
		return nil, err
	}

	ix := entry.ix
	var body []byte
	if len(ix.ResponseBody) > 0 {
		body = ix.ResponseBody
	} else {
		body = []byte(ix.ResponseText)
	}
	header := http.Header{}
	if ix.ContentType != "" {
		header.Set("Content-Type", ix.ContentType)
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", ix.Status, http.StatusText(ix.Status)),
		StatusCode:    ix.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// match finds the recorded interaction for a request. Reads are served in
// recorded order, and the last one keeps being served once they run out.
// Mutations are matched by body against any not-yet-served interaction for the
// same key, since Terraform may apply independent resources in either order.
func (r *recorder) match(key, method string, reqBody []byte) (*replayEntry, error) {
	queue := r.queues[key]
	if len(queue) == 0 {
		return nil, fmt.Errorf("replaying %s: no recorded interaction for %s\n"+
			"The provider made a request the recording doesn't have. If that's expected, re-record with: %s",
			r.name, key, rerecordCommand(r.name))
	}

	if isReadMethod(method) {
		for _, e := range queue {
			if e.remaining > 0 {
				e.remaining--
				return e, nil
			}
		}
		return queue[len(queue)-1], nil
	}

	var closest *replayEntry
	for _, e := range queue {
		if e.remaining == 0 {
			continue
		}
		if closest == nil {
			closest = e
		}
		if requestBodiesEqual(e.ix, reqBody) {
			e.remaining--
			return e, nil
		}
	}
	got, _ := encodeBody(reqBody)
	msg := fmt.Sprintf("replaying %s: no recorded %s with a matching body\n  sent:     %s", r.name, key, got)
	if closest != nil {
		msg += fmt.Sprintf("\n  expected: %s", closest.ix.RequestBody)
	} else {
		msg += "\n  every recorded request for this endpoint was already served"
	}
	return nil, fmt.Errorf("%s\nIf the change is expected, re-record with: %s", msg, rerecordCommand(r.name))
}

// finish saves a recording, or reports replayed mutations that were never
// requested.
func (r *recorder) finish() {
	if r.replay {
		r.reportUnconsumed()
		return
	}
	if r.t.Failed() || r.t.Skipped() {
		r.t.Logf("not saving recording %s because the test didn't pass", r.path)
		return
	}
	if err := r.save(); err != nil {
		r.t.Errorf("saving recording: %v", err)
	}
}

func (r *recorder) reportUnconsumed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	unusedReads := 0
	for _, ix := range r.rec.Interactions {
		for _, e := range r.queues[ix.key()] {
			if e.ix != ix || e.remaining == 0 {
				continue
			}
			if isReadMethod(ix.Method) {
				unusedReads += e.remaining
				continue
			}
			r.t.Errorf("replaying %s: recorded %s was never requested (body: %s)\n"+
				"If the change is expected, re-record with: %s",
				r.name, ix.key(), ix.RequestBody, rerecordCommand(r.name))
		}
	}
	if unusedReads > 0 {
		r.t.Logf("replaying %s: %d recorded reads were never requested", r.name, unusedReads)
	}
}

func (r *recorder) save() error {
	r.mu.Lock()
	data, err := marshalJSON(r.rec, true)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	for _, env := range []string{CockroachAPIKey, CockroachAPIJWT} {
		if secret := os.Getenv(env); secret != "" && bytes.Contains(data, []byte(secret)) {
			return fmt.Errorf("refusing to write %s because it contains the value of %s", r.path, env)
		}
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.path, append(data, '\n'), 0o644)
}

// recorderTransport routes requests through the active recorder. With no
// active recorder it calls inner, except in replay mode, where it never
// touches the network.
type recorderTransport struct {
	inner http.RoundTripper
}

func (rt recorderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := currentRecorder()
	if r == nil {
		if recordingMode() == recordingModeReplay {
			return nil, fmt.Errorf("%s=%s but no recording is active; refusing to call %s %s",
				recordingModeEnvVar, recordingModeReplay, req.Method, req.URL)
		}
		return rt.inner.RoundTrip(req)
	}
	return r.roundTrip(req, rt.inner)
}

// replayRetryContext behaves like retry.RetryContext without sleeping between
// attempts, since replayed responses are already in their final order.
func replayRetryContext(ctx context.Context, timeout time.Duration, f retry.RetryFunc) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var lastErr error
	for range replayMaxRetries {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		rerr := f()
		if rerr == nil {
			return nil
		}
		if !rerr.Retryable {
			return rerr.Err
		}
		lastErr = rerr.Err
	}
	return fmt.Errorf("gave up after %d attempts while replaying recorded API traffic; "+
		"the recording is likely out of date: %w", replayMaxRetries, lastErr)
}

func isReadMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func normalizeQuery(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	return values.Encode()
}

// encodeBody returns a redacted, canonical JSON body, or the raw text when the
// body isn't JSON.
func encodeBody(body []byte) (json.RawMessage, string) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, ""
	}
	v, ok := decodeJSON(body)
	if !ok {
		return nil, string(body)
	}
	data, err := marshalJSON(redact(v), false)
	if err != nil {
		return nil, string(body)
	}
	return data, ""
}

func requestBodiesEqual(ix *interaction, body []byte) bool {
	gotJSON, gotText := encodeBody(body)
	if gotText != "" || ix.RequestText != "" {
		return gotText == ix.RequestText
	}
	if len(gotJSON) == 0 || len(ix.RequestBody) == 0 {
		return len(gotJSON) == len(ix.RequestBody)
	}
	got, _ := decodeJSON(gotJSON)
	want, _ := decodeJSON(ix.RequestBody)
	return reflect.DeepEqual(got, want)
}

func decodeJSON(data []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if dec.More() {
		return nil, false
	}
	return v, true
}

func redact(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			if s, ok := child.(string); ok && s != "" && redactedFields[strings.ToLower(k)] {
				v[k] = redactedValue
				continue
			}
			v[k] = redact(child)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = redact(child)
		}
		return v
	default:
		return v
	}
}

// marshalJSON is json.Marshal without HTML escaping, so certificates and
// connection strings stay readable in recordings.
func marshalJSON(v any, indent bool) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
