package driver

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
)

func TestGCB(t *testing.T) {
	//gcb, err := NewGCB("gcb://puerco-chainguard/80bed495-42c1-4094-be6f-c9eff0bc29df")
	gcb, err := NewGCB("gcb://puerco-chainguard/5dda8a10-abff-4c32-b003-758eea81ac83")

	require.NoError(t, err)
	artifacts, err := gcb.readArtifacts()
	require.NoError(t, err)
	require.Nil(t, artifacts)
}

func TestGCSAttrs(t *testing.T) {
	client, err := storage.NewClient(context.Background())
	require.NoError(t, err)
	attrs, err := readGCSObjectAttributes(client, "gs://puerco-chainguard-public/test-build/7a3bd0e/README.md")
	require.Error(t, err)
	require.NotNil(t, attrs)
}
