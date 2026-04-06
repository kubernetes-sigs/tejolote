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

package attestation

import (
	"encoding/json"
	"testing"
	"time"

	slsa1 "github.com/in-toto/attestation/go/predicates/provenance/v1"
	v1 "github.com/in-toto/attestation/go/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestSLSAPredicateV1MarshalJSON(t *testing.T) {
	pred := NewSLSAV1Predicate()

	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)

	pred.SetBuilderID("https://github.com/actions/runner")
	pred.SetBuilderType("https://actions.github.io/buildtypes/workflow/v1")
	pred.SetInvocationID("https://github.com/example/repo/actions/runs/123/attempts/1")
	pred.SetEntryPoint(".github/workflows/release.yaml")
	pred.SetStartedOn(&now)
	pred.SetFinishedOn(&later)
	pred.SetResolvedDependencies([]*v1.ResourceDescriptor{
		{
			Uri:    "git+ssh://github.com/example/repo@abc123",
			Digest: map[string]string{"sha1": "abc123"},
		},
	})
	pred.SetInternalParameters(map[string]any{
		"github": map[string]any{
			"event_name": "push",
		},
	})

	data, err := json.Marshal(pred)
	require.NoError(t, err)

	// The key test: the output must parse back with protojson into the
	// SLSA v1 Provenance proto. This fails if timestamps are serialized
	// as {"seconds":N} instead of RFC 3339 strings.
	var parsed slsa1.Provenance
	require.NoError(t, protojson.Unmarshal(data, &parsed), "MarshalJSON output must be valid protojson")

	require.Equal(t, "https://actions.github.io/buildtypes/workflow/v1", parsed.GetBuildDefinition().GetBuildType())
	require.Equal(t, "https://github.com/actions/runner", parsed.GetRunDetails().GetBuilder().GetId())
	require.Equal(t, "https://github.com/example/repo/actions/runs/123/attempts/1", parsed.GetRunDetails().GetMetadata().GetInvocationId())
	require.Equal(t, now.Unix(), parsed.GetRunDetails().GetMetadata().GetStartedOn().GetSeconds())
	require.Equal(t, later.Unix(), parsed.GetRunDetails().GetMetadata().GetFinishedOn().GetSeconds())
	require.Len(t, parsed.GetBuildDefinition().GetResolvedDependencies(), 1)
	require.Equal(t, "git+ssh://github.com/example/repo@abc123", parsed.GetBuildDefinition().GetResolvedDependencies()[0].GetUri())
}

func TestSLSAPredicateV1MarshalJSON_NilTimestamps(t *testing.T) {
	pred := NewSLSAV1Predicate()
	pred.SetStartedOn(nil)
	pred.SetFinishedOn(nil)
	pred.SetBuilderID("https://example.com/builder")

	data, err := json.Marshal(pred)
	require.NoError(t, err)

	var parsed slsa1.Provenance
	require.NoError(t, protojson.Unmarshal(data, &parsed), "MarshalJSON output with nil timestamps must be valid protojson")
}

func TestSLSAPredicateV1_AttestationToJSON(t *testing.T) {
	att := New().SLSAv1()

	now := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	later := now.Add(5 * time.Minute)

	att.Predicate.SetBuilderID("https://github.com/actions/runner")
	att.Predicate.SetBuilderType("https://actions.github.io/buildtypes/workflow/v1")
	att.Predicate.SetStartedOn(&now)
	att.Predicate.SetFinishedOn(&later)
	att.Subject = append(att.Subject, &v1.ResourceDescriptor{
		Name:   "example-binary",
		Digest: map[string]string{"sha256": "abcdef1234567890"},
	})

	data, err := att.ToJSON()
	require.NoError(t, err)

	// Parse the full statement back and extract the predicate
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	predData, ok := raw["predicate"]
	require.True(t, ok, "statement must contain a predicate field")

	// The predicate must parse as a valid SLSA v1 provenance proto
	var parsed slsa1.Provenance
	require.NoError(t, protojson.Unmarshal(predData, &parsed),
		"full attestation predicate must be valid protojson")

	require.Equal(t, now.Unix(), parsed.GetRunDetails().GetMetadata().GetStartedOn().GetSeconds())
	require.Equal(t, later.Unix(), parsed.GetRunDetails().GetMetadata().GetFinishedOn().GetSeconds())
}
