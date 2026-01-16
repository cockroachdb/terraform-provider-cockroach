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
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIsRetryableCloudError tests that isRetryableCloudError correctly identifies
// HTTP status codes that indicate cloud IAM propagation issues.
func TestIsRetryableCloudError(t *testing.T) {
	testCases := []struct {
		name        string
		statusCode  int
		expectRetry bool
		description string
	}{
		{
			name:        "400 Bad Request should trigger retry",
			statusCode:  http.StatusBadRequest,
			expectRetry: true,
			description: "KMS permission errors can be returned as 400",
		},
		{
			name:        "401 Unauthorized should trigger retry",
			statusCode:  http.StatusUnauthorized,
			expectRetry: true,
			description: "IAM authentication not yet propagated",
		},
		{
			name:        "403 Forbidden should trigger retry",
			statusCode:  http.StatusForbidden,
			expectRetry: true,
			description: "IAM permission not yet propagated",
		},
		{
			name:        "404 Not Found should not trigger retry",
			statusCode:  http.StatusNotFound,
			expectRetry: false,
			description: "Resource not found - permanent error",
		},
		{
			name:        "409 Conflict should not trigger retry",
			statusCode:  http.StatusConflict,
			expectRetry: false,
			description: "Resource conflict - not IAM related",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.statusCode}
			result := isRetryableCloudError(resp)

			require.Equal(t, tc.expectRetry, result,
				"isRetryableCloudError(%d) = %v, want %v (%s)",
				tc.statusCode, result, tc.expectRetry, tc.description)
		})
	}
}

// TestIsRetryableCloudError_NilResponse tests that nil responses are handled.
func TestIsRetryableCloudError_NilResponse(t *testing.T) {
	result := isRetryableCloudError(nil)
	require.False(t, result, "isRetryableCloudError(nil) should return false")
}

// TestIAMRetryHelper_CheckRetryableCloudError tests the iamRetryHelper behavior.
func TestIAMRetryHelper_CheckRetryableCloudError(t *testing.T) {
	t.Run("non-IAM error returns false with no error", func(t *testing.T) {
		helper := newIAMRetryHelper()
		resp := &http.Response{StatusCode: http.StatusNotFound}

		isRetryable, err := helper.checkRetryableCloudError(resp, "not found")
		require.False(t, isRetryable, "Expected non-IAM error to not be retryable")
		require.NoError(t, err, "Expected no error for non-IAM error")
	})

	t.Run("IAM error after timeout returns non-retryable with error", func(t *testing.T) {
		// Use a very short timeout for testing
		shortTimeout := 50 * time.Millisecond
		defer HookGlobal(&iamPropagationTimeout, shortTimeout)()

		helper := newIAMRetryHelper()
		resp := &http.Response{StatusCode: http.StatusForbidden}

		// First call starts the timer
		isRetryable1, err1 := helper.checkRetryableCloudError(resp, "access denied")
		require.True(t, isRetryable1, "Expected first IAM error to be retryable")
		require.NoError(t, err1, "Expected no error for first IAM error")

		// Wait for timeout to expire
		time.Sleep(shortTimeout + 20*time.Millisecond)

		// Second call after timeout
		isRetryable2, err2 := helper.checkRetryableCloudError(resp, "access denied")
		require.False(t, isRetryable2, "Expected isRetryable=false after timeout")
		require.Error(t, err2, "Expected error after timeout")
		require.Contains(t, err2.Error(), "cloud IAM error persisted",
			"Expected error message to mention cloud IAM error persisted")
		require.Contains(t, err2.Error(), "permanent permission issue",
			"Expected error message to mention permanent permission issue")
	})
}
