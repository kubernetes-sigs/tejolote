package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGCSSnap(t *testing.T) {
	// gs://ulabs-cloud-tests/test/
	gcs, err := NewGCS("gs://kubernetes-release/release/v1.24.4/bin/windows/386/")
	require.NoError(t, err)
	snap, err := gcs.Snap()
	require.Error(t, err)
	require.NotNil(t, snap)
}

func TestSyncGSFile(t *testing.T) {
	gcs, err := NewGCS("gs://kubernetes-release/release/v1.24.4/bin/")

	require.NoError(t, err)
	require.NoError(t, gcs.syncGSFile(context.Background(), "release/v1.24.4/bin/windows/386/kubectl.exe.sha256"))
}
