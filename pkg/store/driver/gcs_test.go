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

package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGCSSnap(t *testing.T) {
	t.Skip("Review this test")
	gcs, err := NewGCS("gs://kubernetes-release/release/v1.24.4/bin/windows/386/")
	require.NoError(t, err)

	snap, err := gcs.Snap()
	require.Error(t, err)
	require.NotNil(t, snap)
}

func TestSyncGSFile(t *testing.T) {
	t.Skip("Review this test")
	gcs, err := NewGCS("gs://kubernetes-release/release/v1.24.4/bin/")
	require.NoError(t, err)
	require.NoError(t, gcs.syncGSFile("release/v1.24.4/bin/windows/386/kubectl.exe.sha256"))
}
