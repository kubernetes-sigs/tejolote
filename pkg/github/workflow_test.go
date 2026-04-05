/*
Copyright 2026 The Kubernetes Authors.

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

package github

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveInputs(t *testing.T) {
	defined := map[string]WorkflowInput{
		"release_tag": {Default: "latest"},
		"dry_run":     {Default: "true"},
		"environment": {Default: ""},
	}

	runInputs := map[string]string{
		"release_tag": "v1.2.3",
		"environment": "prod",
	}

	result := EffectiveInputs(defined, runInputs)
	require.Equal(t, "v1.2.3", result["release_tag"])
	require.Equal(t, "true", result["dry_run"])
	require.Equal(t, "prod", result["environment"])
}

func TestEffectiveInputsNoDefaults(t *testing.T) {
	defined := map[string]WorkflowInput{
		"tag":     {},
		"channel": {Default: "stable"},
	}

	result := EffectiveInputs(defined, nil)

	if _, ok := result["tag"]; ok {
		t.Error("expected tag to be absent (no default, no run input)")
	}
	require.Equal(t, "stable", result["channel"], "expected channel=stable, got %s", result["channel"])
}

func TestFetchWorkflowInputs(t *testing.T) {
	workflowYAML := `
name: Release
on:
  workflow_dispatch:
    inputs:
      release_tag:
        description: "Tag to release"
        required: true
        type: string
      dry_run:
        description: "Dry run mode"
        default: "true"
        type: boolean
  workflow_call:
    inputs:
      caller_input:
        description: "Input from caller"
        default: "default_val"
        type: string
`
	encoded := base64.StdEncoding.EncodeToString([]byte(workflowYAML))
	resp := contentsResponse{Content: encoded, Encoding: "base64"}
	respJSON, err := json.Marshal(resp)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respJSON)
	}))
	defer server.Close()

	// Override the API URL by setting up the test server
	origURL := ghContentsURL
	ghContentsURL = server.URL + "/%s/%s/%s?ref=%s"
	defer func() { ghContentsURL = origURL }()

	// Set GITHUB_TOKEN to avoid auth warnings in tests
	t.Setenv("GITHUB_TOKEN", "test-token")

	inputs, err := FetchWorkflowInputs("org", "repo", ".github/workflows/release.yml", "abc123")
	require.NoError(t, err)

	require.Len(t, inputs, 3, "expected 3 inputs, got %d", len(inputs))
	require.Equal(t, "string", inputs["release_tag"].Type, "expected release_tag type=string, got %s", inputs["release_tag"].Type)
	require.Equal(t, "true", inputs["dry_run"].Default, "expected dry_run default=true, got %s", inputs["dry_run"].Default)
	require.Equal(t, "default_val", inputs["caller_input"].Default, "expected caller_input default=default_val, got %s", inputs["caller_input"].Default)
}
