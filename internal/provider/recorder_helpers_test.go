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
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	// recordingModeEnvVar selects how acceptance tests that call
	// accTestSetup talk to the API. Unset means live, against the real API.
	recordingModeEnvVar = "COCKROACH_TEST_RECORDING"
	// recordingModeRecord runs against the real API and saves the traffic.
	recordingModeRecord = "record"
	// recordingModeReplay serves the saved traffic and never calls the API.
	recordingModeReplay = "replay"

	recordingsDir = "testdata/recordings"
)

// pinnedTestVars are package vars that are read from the environment, so they
// are saved with a recording and restored on replay.
var pinnedTestVars = map[string]*string{
	"testRegion":       &testRegion,
	"testSecondRegion": &testSecondRegion,
	"testThirdRegion":  &testThirdRegion,
}

var (
	activeRecorderMu sync.Mutex
	activeRecorder   *recorder
)

func recordingMode() string {
	return os.Getenv(recordingModeEnvVar)
}

// installRecordingHooks routes the provider's HTTP traffic through
// recorderTransport when a recording mode is set.
func installRecordingHooks() {
	switch mode := recordingMode(); mode {
	case "":
		return
	case recordingModeRecord:
	case recordingModeReplay:
		retryContext = replayRetryContext
	default:
		panic(fmt.Sprintf("%s must be %q, %q or unset, got %q",
			recordingModeEnvVar, recordingModeRecord, recordingModeReplay, mode))
	}
	wrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return recorderTransport{inner: rt}
	}
}

func currentRecorder() *recorder {
	activeRecorderMu.Lock()
	defer activeRecorderMu.Unlock()
	return activeRecorder
}

func recordingPath(testName string) string {
	return filepath.Join(recordingsDir, strings.ReplaceAll(testName, "/", "__")+".json")
}

func rerecordCommand(testName string) string {
	return fmt.Sprintf("make testrecord RUN='^%s$'", testName)
}

// accTestSetup must be the first call in an acceptance test that supports
// recording. In live mode it marks the test parallel. In record and replay
// mode the test runs sequentially, because the provider and API client are
// shared package globals and traffic can only be attributed to one test at a
// time.
func accTestSetup(t *testing.T) {
	t.Helper()
	mode := recordingMode()
	if mode == "" {
		t.Parallel()
		return
	}
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skipf("%s=%s requires %s to be set", recordingModeEnvVar, mode, resource.EnvTfAcc)
	}

	path := recordingPath(t.Name())
	var r *recorder
	if mode == recordingModeReplay {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			t.Skipf("no recording at %s; create one with: %s", path, rerecordCommand(t.Name()))
		}
		var err error
		if r, err = newReplayingRecorder(t, path); err != nil {
			t.Fatal(err)
		}
		if os.Getenv(CockroachAPIKey) == "" && os.Getenv(CockroachAPIJWT) == "" {
			t.Setenv(CockroachAPIKey, "replay")
		}
	} else {
		r = newRecordingRecorder(t, path)
	}

	activeRecorderMu.Lock()
	if activeRecorder != nil {
		other := activeRecorder.name
		activeRecorderMu.Unlock()
		t.Fatalf("%s is already recording; tests can't run in parallel with %s set", other, recordingModeEnvVar)
	}
	activeRecorder = r
	activeRecorderMu.Unlock()

	restoreVars := pinTestVars(r)
	t.Cleanup(func() {
		activeRecorderMu.Lock()
		activeRecorder = nil
		activeRecorderMu.Unlock()
		restoreVars()
		r.finish()
	})
}

// pinTestVars saves pinnedTestVars to a new recording, or restores them from
// a replayed one.
func pinTestVars(r *recorder) (restore func()) {
	orig := map[string]string{}
	for name, ptr := range pinnedTestVars {
		orig[name] = *ptr
		if !r.replay {
			r.rec.Vars[name] = *ptr
		} else if v, ok := r.rec.Vars[name]; ok {
			*ptr = v
		}
	}
	return func() {
		for name, ptr := range pinnedTestVars {
			*ptr = orig[name]
		}
	}
}

// testNow returns the current time, or the recording's time when one is
// active, so time-based test inputs match on replay.
func testNow() time.Time {
	if r := currentRecorder(); r != nil {
		return r.rec.Now
	}
	return time.Now()
}
