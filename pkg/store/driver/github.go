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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	gogithub "github.com/google/go-github/v88/github"
	intoto "github.com/in-toto/attestation/go/v1"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/release-sdk/github"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

// ErrReleaseNotFound is returned when a repository has no release for the tag.
var ErrReleaseNotFound = errors.New("release not found")

type GitHubRelease struct {
	Owner      string
	Repository string
	Tag        string
	Options    GitHubReleaseOptions
	client     github.Client
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
		client:     github.New().Client(),
	}

	return ghr, nil
}

// Snap captures the assets published in the release as a snapshot. Each asset
// becomes an entry keyed by its name, checksummed with the digest GitHub
// reports for it. Assets with no digest are downloaded to be hashed locally.
func (ghr *GitHubRelease) Snap() (*snapshot.Snapshot, error) {
	ctx := context.Background()

	release, err := ghr.getRelease(ctx)
	if err != nil {
		return nil, err
	}

	snap := snapshot.Snapshot{}
	var mtx sync.Mutex
	var wg errgroup.Group
	wg.SetLimit(4)

	for _, asset := range release.Assets {
		name := asset.GetName()
		switch {
		case name == "":
			logrus.Warnf("skipping unnamed asset %d in release %s", asset.GetID(), ghr.Tag)
			continue
		case ghr.ignoreAsset(name):
			logrus.Debugf("skipping asset %s, its extension is ignored", name)
			continue
		case asset.GetState() != "" && asset.GetState() != "uploaded":
			// Maybe we should wait here?
			logrus.Warnf("skipping asset %s, its state is %q", name, asset.GetState())
			continue
		}

		wg.Go(func() error {
			csum, err := ghr.assetChecksum(ctx, asset)
			if err != nil {
				return fmt.Errorf("checksumming asset %s: %w", name, err)
			}

			mtx.Lock()
			defer mtx.Unlock()
			snap[name] = run.Artifact{
				Path: name,
				URL:  asset.GetBrowserDownloadURL(),
				Checksum: map[string]string{
					string(intoto.AlgorithmSHA256): csum,
				},
				Time: asset.GetUpdatedAt().Time,
			}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, fmt.Errorf("reading release assets: %w", err)
	}

	logrus.Infof(
		"collected %d assets from release %s of %s/%s",
		len(snap), ghr.Tag, ghr.Owner, ghr.Repository,
	)
	return &snap, nil
}

// getRelease returns the release data for the driver's tag.
//
// Draft releases are not published under their tag, the by-tag endpoint returns
// a 404 for them. Since drafts are a normal stage of a release run (artifacts
// are commonly attested while the release is still being assembled) we fall
// back to looking for the tag in the repository release list, which does return
// drafts to callers that can see them.
func (ghr *GitHubRelease) getRelease(ctx context.Context) (*gogithub.RepositoryRelease, error) {
	release, resp, err := ghr.client.GetReleaseByTag(ctx, ghr.Owner, ghr.Repository, ghr.Tag)
	if err == nil {
		return release, nil
	}

	if resp == nil || resp.StatusCode != http.StatusNotFound {
		return nil, fmt.Errorf(
			"getting release %s from %s/%s: %w", ghr.Tag, ghr.Owner, ghr.Repository, err,
		)
	}

	logrus.Debugf(
		"no published release tagged %s, looking for a draft in the release list", ghr.Tag,
	)
	return ghr.findReleaseInList(ctx)
}

// findReleaseInList looks for a release matching the specified tag by paging
// through the repository releases (which actually lists drafts)
func (ghr *GitHubRelease) findReleaseInList(ctx context.Context) (*gogithub.RepositoryRelease, error) {
	opts := &gogithub.ListOptions{PerPage: 100}
	for page := 1; page <= 10; page++ {
		opts.Page = page
		releases, _, err := ghr.client.ListReleases(ctx, ghr.Owner, ghr.Repository, opts)
		if err != nil {
			return nil, fmt.Errorf("listing releases of %s/%s: %w", ghr.Owner, ghr.Repository, err)
		}

		for _, release := range releases {
			if release.GetTagName() != ghr.Tag {
				continue
			}
			if release.GetDraft() {
				logrus.Infof("release %s is still a draft, attesting its assets", ghr.Tag)
			}
			return release, nil
		}

		if len(releases) < 100 {
			break
		}
	}

	return nil, fmt.Errorf(
		"%w: no release tagged %s in %s/%s", ErrReleaseNotFound, ghr.Tag, ghr.Owner, ghr.Repository,
	)
}

// assetChecksum returns the sha256 of a release asset. GitHub records the
// digest of the assets it stores, so we don't need to download them to hash.
// Only if they don't have hashes we download them as before.
func (ghr *GitHubRelease) assetChecksum(ctx context.Context, asset *gogithub.ReleaseAsset) (string, error) {
	if csum, ok := sha256FromDigest(asset.GetDigest()); ok {
		logrus.Debugf("asset %s: using digest reported by the API", asset.GetName())
		return csum, nil
	}

	logrus.Infof("asset %s has no digest in the API, downloading to hash", asset.GetName())
	data, redirect, err := ghr.client.DownloadReleaseAsset(ctx, ghr.Owner, ghr.Repository, asset.GetID())
	if err != nil {
		return "", fmt.Errorf("downloading asset: %w", err)
	}
	if data == nil {
		return "", fmt.Errorf("no data returned for asset (redirected to %q)", redirect)
	}
	defer data.Close()

	shaVal := sha256.New()
	if _, err := io.Copy(shaVal, data); err != nil {
		return "", fmt.Errorf("hashing asset data: %w", err)
	}
	return hex.EncodeToString(shaVal.Sum(nil)), nil
}

// sha256FromDigest reads the digest GitHub reports for a release asset. It
// returns the hex encoded hash and true when the value is a well formed sha256.
func sha256FromDigest(digest string) (string, bool) {
	algo, value, ok := strings.Cut(digest, ":")
	if !ok || !strings.EqualFold(algo, string(intoto.AlgorithmSHA256)) {
		return "", false
	}

	value = strings.ToLower(value)
	if len(value) != 64 {
		return "", false
	}
	// Check to see if its really an encoded sha256
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return value, true
}

// ignoreAsset returns true when an asset should be left out of the snapshot
// because of its extension.
func (ghr *GitHubRelease) ignoreAsset(name string) bool {
	for _, ext := range ghr.Options.IgnoreExtensions {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
