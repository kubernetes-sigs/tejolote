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

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
)

func TestGCB(t *testing.T) {
	t.Skip("Review this test")
	gcb, err := NewGCB("gcb://puerco-chainguard/5dda8a10-abff-4c32-b003-758eea81ac83")
	require.NoError(t, err)

	artifacts, err := gcb.readArtifacts()
	require.NoError(t, err)
	require.Nil(t, artifacts)
}

func TestGCSAttrs(t *testing.T) {
	t.Skip("Review this test")
	client, err := storage.NewClient(t.Context())
	require.NoError(t, err)

	attrs, err := readGCSObjectAttributes(client, "gs://puerco-chainguard-public/test-build/7a3bd0e/README.md")
	require.Error(t, err)
	require.NotNil(t, attrs)
}
