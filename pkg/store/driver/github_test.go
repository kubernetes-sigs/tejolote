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
	"strings"
	"sync"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v90/github"
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
		(*snap)["sbom.spdx"].Checksum["sha256"],
	)
}

const (
	testOwner = "test"
	testRepo  = "repo"
)

// testRelease returns a release carrying the given assets.
func testRelease(tag string, draft bool, assets ...*gogithub.ReleaseAsset) *gogithub.RepositoryRelease {
	return &gogithub.RepositoryRelease{
		TagName: tag,
		Draft:   draft,
		Assets:  assets,
	}
}

// testAsset returns a release asset. An empty digest models an asset stored
// before GitHub started recording them.
func testAsset(id int64, name, digest string) *gogithub.ReleaseAsset {
	asset := &gogithub.ReleaseAsset{
		ID:                 gogithub.Ptr(id),
		Name:               gogithub.Ptr(name),
		State:              gogithub.Ptr("uploaded"),
		BrowserDownloadURL: gogithub.Ptr("https://github.com/test/repo/releases/download/v1.0.0/" + name),
		UpdatedAt:          &gogithub.Timestamp{Time: time.Unix(1700000000, 0)},
	}
	if digest != "" {
		asset.Digest = gogithub.Ptr(digest)
	}
	return asset
}

// notFound returns the response and error the API returns for a release that
// cannot be read through the by-tag endpoint, which is the case for drafts.
func notFound() (*gogithub.Response, error) {
	return &gogithub.Response{
			Response: &http.Response{StatusCode: http.StatusNotFound},
		}, &gogithub.ErrorResponse{
			Response: &http.Response{StatusCode: http.StatusNotFound},
			Message:  "Not Found",
		}
}

func sha256Of(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

type listCall struct {
	owner, repo string
	page        int
}

type downloadCall struct {
	owner, repo string
	id          int64
}

// fakeReleaseClient serves canned API responses to the driver and records the
// calls it receives.
type fakeReleaseClient struct {
	// release is what the by-tag endpoint answers with
	release     *gogithub.RepositoryRelease
	releaseResp *gogithub.Response
	releaseErr  error

	// pages are the release list pages, indexed from page 1
	pages   [][]*gogithub.RepositoryRelease
	listErr error

	// assetData is the payload served for any asset download
	assetData string

	mtx       sync.Mutex
	listCalls []listCall
	downloads []downloadCall
}

func (f *fakeReleaseClient) GetReleaseByTag(
	_ context.Context, _, _, _ string,
) (*gogithub.RepositoryRelease, *gogithub.Response, error) {
	return f.release, f.releaseResp, f.releaseErr
}

func (f *fakeReleaseClient) ListReleases(
	_ context.Context, owner, repo string, opts *gogithub.ListOptions,
) ([]*gogithub.RepositoryRelease, *gogithub.Response, error) {
	f.mtx.Lock()
	f.listCalls = append(f.listCalls, listCall{owner: owner, repo: repo, page: opts.Page})
	f.mtx.Unlock()

	if f.listErr != nil {
		return nil, nil, f.listErr
	}
	if opts.Page < 1 || opts.Page > len(f.pages) {
		return nil, nil, nil
	}
	return f.pages[opts.Page-1], nil, nil
}

func (f *fakeReleaseClient) DownloadReleaseAsset(
	_ context.Context, owner, repo string, id int64, _ *http.Client,
) (io.ReadCloser, string, error) {
	f.mtx.Lock()
	f.downloads = append(f.downloads, downloadCall{owner: owner, repo: repo, id: id})
	f.mtx.Unlock()

	return io.NopCloser(strings.NewReader(f.assetData)), "", nil
}

// testDriver returns a driver wired to the given fake client.
func testDriver(fake *fakeReleaseClient) *GitHubRelease {
	return &GitHubRelease{
		Owner:      testOwner,
		Repository: testRepo,
		Tag:        "v1.0.0",
		Options:    DefaultGitHubReleaseOptions,
		client:     fake,
	}
}

func TestSnapUsesDigestReportedByTheAPI(t *testing.T) {
	digest := sha256Of("release artifact")
	fake := &fakeReleaseClient{
		release: testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "sha256:"+digest)),
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Len(t, *snap, 1)
	require.Equal(t, digest, (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(
		t, "https://github.com/test/repo/releases/download/v1.0.0/artifact.zip",
		(*snap)["artifact.zip"].URL,
	)
	require.Equal(t, time.Unix(1700000000, 0), (*snap)["artifact.zip"].Time)

	// The digest is enough, no asset data should have been transferred
	require.Empty(t, fake.downloads)
}

func TestSnapReadsDraftReleases(t *testing.T) {
	digest := sha256Of("draft artifact")
	resp, err := notFound()
	fake := &fakeReleaseClient{
		releaseResp: resp,
		releaseErr:  err,
		pages: [][]*gogithub.RepositoryRelease{{
			testRelease("v2.0.0", false, testAsset(9, "other.zip", "sha256:"+sha256Of("other"))),
			testRelease("v1.0.0", true, testAsset(1, "artifact.zip", "sha256:"+digest)),
		}},
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Len(t, *snap, 1)
	require.Equal(t, digest, (*snap)["artifact.zip"].Checksum["sha256"])
	require.Len(t, fake.listCalls, 1)
}

func TestSnapPagesThroughReleases(t *testing.T) {
	digest := sha256Of("artifact on the second page")
	firstPage := make([]*gogithub.RepositoryRelease, 100)
	for i := range firstPage {
		firstPage[i] = testRelease(fmt.Sprintf("v0.0.%d", i), false)
	}

	resp, err := notFound()
	fake := &fakeReleaseClient{
		releaseResp: resp,
		releaseErr:  err,
		pages: [][]*gogithub.RepositoryRelease{
			firstPage,
			{testRelease("v1.0.0", true, testAsset(1, "artifact.zip", "sha256:"+digest))},
		},
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, digest, (*snap)["artifact.zip"].Checksum["sha256"])
	require.Len(t, fake.listCalls, 2)
	require.Equal(t, listCall{owner: testOwner, repo: testRepo, page: 2}, fake.listCalls[1])
}

func TestSnapStopsPagingWhenTheReleaseIsMissing(t *testing.T) {
	resp, err := notFound()
	fake := &fakeReleaseClient{
		releaseResp: resp,
		releaseErr:  err,
		pages:       [][]*gogithub.RepositoryRelease{{testRelease("v2.0.0", false)}},
	}

	_, err = testDriver(fake).Snap()
	require.ErrorIs(t, err, ErrReleaseNotFound)
	// A short page means there is nothing else to read
	require.Len(t, fake.listCalls, 1)
}

func TestSnapDoesNotFallBackOnOtherAPIErrors(t *testing.T) {
	fake := &fakeReleaseClient{
		releaseResp: &gogithub.Response{
			Response: &http.Response{StatusCode: http.StatusInternalServerError},
		},
		releaseErr: errors.New("boom"),
	}

	_, err := testDriver(fake).Snap()
	require.Error(t, err)
	require.Empty(t, fake.listCalls)
}

func TestSnapHashesAssetsWithoutDigest(t *testing.T) {
	data := "artifact stored before github recorded digests"
	fake := &fakeReleaseClient{
		release:   testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "")),
		assetData: data,
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, sha256Of(data), (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(t, []downloadCall{{owner: testOwner, repo: testRepo, id: 1}}, fake.downloads)
}

func TestSnapHashesAssetsWithUnusableDigest(t *testing.T) {
	data := "artifact digested with something else"
	fake := &fakeReleaseClient{
		release:   testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "sha512:abc")),
		assetData: data,
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, sha256Of(data), (*snap)["artifact.zip"].Checksum["sha256"])
	require.Len(t, fake.downloads, 1)
}

func TestSnapSkipsIgnoredAndIncompleteAssets(t *testing.T) {
	uploading := testAsset(3, "uploading.zip", "sha256:"+sha256Of("uploading"))
	uploading.State = gogithub.Ptr("starter")

	fake := &fakeReleaseClient{
		release: testRelease(
			"v1.0.0", false,
			testAsset(1, "artifact.zip", "sha256:"+sha256Of("artifact")),
			testAsset(2, "artifact.zip.sig", "sha256:"+sha256Of("signature")),
			uploading,
		),
	}

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Len(t, *snap, 1)
	require.Contains(t, *snap, "artifact.zip")
	require.Empty(t, fake.downloads)
}

func TestSha256FromDigest(t *testing.T) {
	valid := sha256Of("data")
	for _, tc := range []struct {
		name     string
		digest   string
		expected string
		ok       bool
	}{
		{"valid", "sha256:" + valid, valid, true},
		{"uppercase algorithm", "SHA256:" + valid, valid, true},
		{"uppercase value", "sha256:" + strings.ToUpper(valid), valid, true},
		{"empty", "", "", false},
		{"no algorithm", valid, "", false},
		{"other algorithm", "sha512:" + valid, "", false},
		{"short value", "sha256:abcdef", "", false},
		{"not hex", "sha256:" + strings.Repeat("z", 64), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			csum, ok := sha256FromDigest(tc.digest)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.expected, csum)
		})
	}
}
