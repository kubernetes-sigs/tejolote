/*
Copyright 2023 The Kubernetes Authors.

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

package driver

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

func TestGitHubRelease(t *testing.T) {
	gh, err := NewGithub("github://puerco/hello/v0.0.1")
	require.NoError(t, err)
	snap, err := gh.Snap()
	require.NoError(t, err)
	require.NotNil(t, snap)
	ns := snapshot.Snapshot{}
	require.Len(t, ns.Delta(snap), 1)
	logrus.Infof("%+v", snap)
	require.Equal(
		t, "2dcb1895edab89c32a356e437d3c94e83fc6cde2d5a052f2e7b4051326f9ba44",
		(*snap)["sbom.spdx"].Checksum["SHA256"],
	)
}
