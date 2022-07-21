/*
Copyright 2022 Adolfo García Veytia

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

package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceURL(t *testing.T) {
	configData := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true

[remote "origin"]
	url = git@github.com:kubernetes-sigs/tejolote.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	tmpdir, err := os.MkdirTemp("", "")
	require.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	// Write a minimal git config to check the remote
	require.NoError(t, os.Mkdir(filepath.Join(tmpdir, ".git"), os.FileMode(0o755)))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpdir, ".git", "config"), []byte(configData), os.FileMode(0o644),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(tmpdir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), os.FileMode(0o644),
	))

	repo := NewRepository(tmpdir)
	url, err := repo.SourceURL()
	require.NoError(t, err)
	require.Equal(t, url, "git@github.com:kubernetes-sigs/tejolote.git")
}
