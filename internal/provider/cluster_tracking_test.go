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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRecordClusterCreation_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	recordClusterCreation(filePath, "id-1", "cluster-one")
	recordClusterCreation(filePath, "id-2", "cluster-two")

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "id-1 cluster-one" {
		t.Errorf("Expected first line 'id-1 cluster-one', got %q", lines[0])
	}
	if lines[1] != "id-2 cluster-two" {
		t.Errorf("Expected second line 'id-2 cluster-two', got %q", lines[1])
	}
}

func TestRecordClusterDeletion_RemovesFromFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	recordClusterCreation(filePath, "id-1", "cluster-one")
	recordClusterCreation(filePath, "id-2", "cluster-two")
	recordClusterCreation(filePath, "id-3", "cluster-three")

	recordClusterDeletion(filePath, "id-2")

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 lines after deletion, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "id-1 cluster-one" {
		t.Errorf("Expected first line 'id-1 cluster-one', got %q", lines[0])
	}
	if lines[1] != "id-3 cluster-three" {
		t.Errorf("Expected second line 'id-3 cluster-three', got %q", lines[1])
	}
}

func TestRecordClusterDeletion_AllRemoved(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	recordClusterCreation(filePath, "id-1", "cluster-one")
	recordClusterDeletion(filePath, "id-1")

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	content := strings.TrimSpace(string(data))
	if content != "" {
		t.Errorf("Expected empty file after all deletions, got %q", content)
	}
}

func TestRecordClusterDeletion_NonexistentID(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	recordClusterCreation(filePath, "id-1", "cluster-one")
	recordClusterDeletion(filePath, "id-nonexistent")

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line (original entry preserved), got %d: %q", len(lines), string(data))
	}
	if lines[0] != "id-1 cluster-one" {
		t.Errorf("Expected 'id-1 cluster-one', got %q", lines[0])
	}
}

func TestRecordClusterDeletion_FileDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "nonexistent.txt")

	recordClusterDeletion(filePath, "id-1")
}

func TestRecordClusterCreation_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	var wg sync.WaitGroup
	count := 20
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := strings.Replace(strings.Repeat("0", 8), "0", string(rune('a'+n%26)), -1)
			recordClusterCreation(filePath, id, "cluster-"+id)
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != count {
		t.Errorf("Expected %d lines from concurrent writes, got %d", count, len(lines))
	}
}

func TestRecordClusterCreationAndDeletion_ConcurrentMixed(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")

	for i := 0; i < 10; i++ {
		recordClusterCreation(
			filePath,
			strings.Replace("id-00", "00", strings.Repeat(string(rune('0'+i)), 2), 1),
			"cluster-"+string(rune('a'+i)),
		)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i += 2 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			recordClusterDeletion(
				filePath,
				strings.Replace("id-00", "00", strings.Repeat(string(rune('0'+n)), 2), 1),
			)
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read tracking file: %s", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Errorf("Expected 5 lines after deleting 5 of 10, got %d: %q", len(lines), string(data))
	}
}

func TestNewTrackingService_DisabledWhenNoEnvVar(t *testing.T) {
	t.Setenv(clusterTrackingEnvVar, "")
	inner := NewService(nil)
	svc := newTrackingService(inner)
	if _, ok := svc.(*trackingService); ok {
		t.Error("Expected unwrapped service when tracking is disabled")
	}
}

func TestNewTrackingService_EnabledWhenEnvVarSet(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "tracking.txt")
	t.Setenv(clusterTrackingEnvVar, filePath)
	trackingInitialized = false

	inner := NewService(nil)
	svc := newTrackingService(inner)
	ts, ok := svc.(*trackingService)
	if !ok {
		t.Fatal("Expected trackingService wrapper when tracking is enabled")
	}
	if ts.filePath != filePath {
		t.Errorf("Expected filePath %q, got %q", filePath, ts.filePath)
	}
}
