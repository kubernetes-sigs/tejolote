/*
Copyright 2022 The Kubernetes Authors.

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

package watcher

import (
	"testing"

	"sigs.k8s.io/tejolote/pkg/run"
)

func TestFilterArtifactsByName(t *testing.T) {
	const (
		amd64Tarball = "release-amd64.tar.gz"
		arm64Tarball = "release-arm64.tar.gz"
		sbom         = "dist/bom.spdx"
		logFile      = "debug.log"
		releaseGlob  = "release-*"
	)
	artifacts := []run.Artifact{
		{Path: amd64Tarball},
		{Path: arm64Tarball},
		{Path: sbom},
		{Path: logFile},
	}

	for _, tc := range []struct {
		name      string
		globs     []string
		expected  []string
		shouldErr bool
	}{
		{name: "no globs keeps everything", globs: nil, expected: []string{
			amd64Tarball, arm64Tarball, sbom, logFile,
		}},
		{name: "single glob", globs: []string{releaseGlob}, expected: []string{
			amd64Tarball, arm64Tarball,
		}},
		{name: "multiple globs match any", globs: []string{releaseGlob, "*.spdx"}, expected: []string{
			amd64Tarball, arm64Tarball, sbom,
		}},
		{name: "glob matches base name not full path", globs: []string{"bom.*"}, expected: []string{
			sbom,
		}},
		{name: "no matches", globs: []string{"nothing-*"}, expected: []string{}},
		{name: "invalid glob errors", globs: []string{releaseGlob, "[bad"}, shouldErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := filterArtifactsByName(artifacts, tc.globs)
			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(res) != len(tc.expected) {
				t.Fatalf("expected %d artifacts, got %d: %+v", len(tc.expected), len(res), res)
			}
			for i, a := range res {
				if a.Path != tc.expected[i] {
					t.Errorf("artifact #%d: expected %q, got %q", i, tc.expected[i], a.Path)
				}
			}
		})
	}
}
