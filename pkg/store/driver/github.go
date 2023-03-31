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
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/release-sdk/github"
	"sigs.k8s.io/release-utils/hash"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type GitHubRelease struct {
	Owner      string
	Repository string
	Tag        string
	Options    GitHubReleaseOptions
	gh         *github.GitHub
}

type GitHubReleaseOptions struct {
	IgnoreExtensions []string
}

var DefaultGitHubReleaseOptions = GitHubReleaseOptions{
	IgnoreExtensions: []string{".pem", ".sig", ".cert"},
}

func NewGithub(specURL string) (*GitHubRelease, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing github spec url: %w", err)
	}

	if u.Scheme != "github" {
		return nil, errors.New("spec url is not a github release url")
	}

	repoTag := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/")
	parts := strings.Split(repoTag, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("unable to find repo/tag in %s", u.Path)
	}

	ghr := &GitHubRelease{
		Owner:      u.Hostname(),
		Repository: parts[0],
		Tag:        parts[1],
		Options:    DefaultGitHubReleaseOptions,
		gh:         github.New(),
	}

	return ghr, nil
}

func (ghr *GitHubRelease) Snap() (*snapshot.Snapshot, error) {
	// Download assets to temporary directory
	tmp, err := os.MkdirTemp("", "github-assets-")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	if err := ghr.gh.DownloadReleaseAssets(
		ghr.Owner, ghr.Repository, []string{ghr.Tag}, tmp,
	); err != nil {
		return nil, fmt.Errorf("downloading release assets: %w", err)
	}

	// Hash EVERYTHING
	snap := snapshot.Snapshot{}
	var mtx sync.Mutex
	if err := filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		hashValue, err := hash.SHA256ForFile(path)
		if err != nil {
			return fmt.Errorf("hashing artifact: %w", err)
		}

		for _, ext := range ghr.Options.IgnoreExtensions {
			if strings.HasSuffix(path, ext) {
				return nil
			}
		}

		mtx.Lock()
		snap[filepath.Base(path)] = run.Artifact{
			Path: filepath.Base(path),
			Checksum: map[string]string{
				"SHA256": hashValue,
			},
			Time: time.Now(), // TODO: This needs to be set properly for future
		}
		mtx.Unlock()
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walking path: %w", err)
	}
	return &snap, nil
}
