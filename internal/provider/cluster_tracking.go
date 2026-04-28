/*
Copyright 2023 The Cockroach Authors

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
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cockroachdb/cockroach-cloud-sdk-go/v7/pkg/client"
)

const clusterTrackingEnvVar = "CLEANUP_TRACKING_FILE"

var (
	trackingMu          sync.Mutex
	trackingInitialized bool
)

func getTrackingFilePath() string {
	return os.Getenv(clusterTrackingEnvVar)
}

// newTrackingService wraps a client.Service to track cluster creation and
// deletion in a file. Returns the original service unwrapped if tracking
// is not enabled (CLEANUP_TRACKING_FILE not set).
func newTrackingService(inner client.Service) client.Service {
	filePath := getTrackingFilePath()
	if filePath == "" {
		return inner
	}

	trackingMu.Lock()
	if !trackingInitialized {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Tracking enabled: file=%s\n", filePath)
		trackingInitialized = true
	}
	trackingMu.Unlock()

	return &trackingService{Service: inner, filePath: filePath}
}

// trackingService wraps client.Service to intercept CreateCluster and
// DeleteCluster calls, recording cluster IDs in a tracking file for
// post-test cleanup.
type trackingService struct {
	client.Service
	filePath string
}

func (s *trackingService) CreateCluster(
	ctx context.Context, req *client.CreateClusterRequest,
) (*client.Cluster, *http.Response, error) {
	cluster, resp, err := s.Service.CreateCluster(ctx, req)
	if err == nil && cluster != nil {
		recordClusterCreation(s.filePath, cluster.GetId(), cluster.GetName())
	}
	return cluster, resp, err
}

func (s *trackingService) DeleteCluster(
	ctx context.Context, clusterId string,
) (*client.Cluster, *http.Response, error) {
	cluster, resp, err := s.Service.DeleteCluster(ctx, clusterId)
	if err == nil {
		recordClusterDeletion(s.filePath, clusterId)
	} else if resp != nil && resp.StatusCode == http.StatusNotFound {
		recordClusterDeletion(s.filePath, clusterId)
	}
	return cluster, resp, err
}

// recordClusterCreation appends a cluster ID and name to the tracking file.
// Best-effort: errors are logged but never fatal.
func recordClusterCreation(filePath, id, name string) {
	trackingMu.Lock()
	defer trackingMu.Unlock()

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error opening tracking file: %s\n", err)
		return
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s %s\n", id, name); err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error writing to tracking file: %s\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "[cluster-tracking] Created: id=%s name=%s\n", id, name)
}

// recordClusterDeletion removes a cluster ID from the tracking file.
// Uses atomic write (temp file + rename) to avoid corruption if the
// process is killed mid-write.
// Best-effort: errors are logged but never fatal.
func recordClusterDeletion(filePath, id string) {
	trackingMu.Lock()
	defer trackingMu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error reading tracking file: %s\n", err)
		return
	}

	var remaining []string
	removed := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, id+" ") || line == id {
			removed = true
			continue
		}
		remaining = append(remaining, line)
	}

	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, "tracking-*.tmp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error creating temp file: %s\n", err)
		return
	}
	tmpPath := tmpFile.Name()

	content := ""
	if len(remaining) > 0 {
		content = strings.Join(remaining, "\n") + "\n"
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error writing temp file: %s\n", err)
		tmpFile.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmpFile.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error closing temp file: %s\n", err)
		os.Remove(tmpPath)
		return
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Error renaming temp file: %s\n", err)
		os.Remove(tmpPath)
		return
	}

	if removed {
		fmt.Fprintf(os.Stderr, "[cluster-tracking] Deleted: id=%s\n", id)
	}
}
