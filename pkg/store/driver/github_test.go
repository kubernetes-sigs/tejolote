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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/release-sdk/github/githubfakes"
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

// testRelease returns a release carrying the given assets.
func testRelease(tag string, draft bool, assets ...*gogithub.ReleaseAsset) *gogithub.RepositoryRelease {
	return &gogithub.RepositoryRelease{
		TagName: gogithub.Ptr(tag),
		Draft:   gogithub.Ptr(draft),
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

// testDriver returns a driver wired to the given fake client.
func testDriver(fake *githubfakes.FakeClient) *GitHubRelease {
	return &GitHubRelease{
		Owner:      "test",
		Repository: "repo",
		Tag:        "v1.0.0",
		Options:    DefaultGitHubReleaseOptions,
		client:     fake,
	}
}

func TestSnapUsesDigestReportedByTheAPI(t *testing.T) {
	digest := sha256Of("release artifact")
	fake := &githubfakes.FakeClient{}
	fake.GetReleaseByTagReturns(
		testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "sha256:"+digest)), nil, nil,
	)

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
	require.Equal(t, 0, fake.DownloadReleaseAssetCallCount())
}

func TestSnapReadsDraftReleases(t *testing.T) {
	digest := sha256Of("draft artifact")
	fake := &githubfakes.FakeClient{}
	resp, err := notFound()
	fake.GetReleaseByTagReturns(nil, resp, err)
	fake.ListReleasesReturns([]*gogithub.RepositoryRelease{
		testRelease("v2.0.0", false, testAsset(9, "other.zip", "sha256:"+sha256Of("other"))),
		testRelease("v1.0.0", true, testAsset(1, "artifact.zip", "sha256:"+digest)),
	}, nil, nil)

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Len(t, *snap, 1)
	require.Equal(t, digest, (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(t, 1, fake.ListReleasesCallCount())
}

func TestSnapPagesThroughReleases(t *testing.T) {
	digest := sha256Of("artifact on the second page")
	firstPage := make([]*gogithub.RepositoryRelease, 100)
	for i := range firstPage {
		firstPage[i] = testRelease(fmt.Sprintf("v0.0.%d", i), false)
	}

	fake := &githubfakes.FakeClient{}
	resp, err := notFound()
	fake.GetReleaseByTagReturns(nil, resp, err)
	fake.ListReleasesReturnsOnCall(0, firstPage, nil, nil)
	fake.ListReleasesReturnsOnCall(1, []*gogithub.RepositoryRelease{
		testRelease("v1.0.0", true, testAsset(1, "artifact.zip", "sha256:"+digest)),
	}, nil, nil)

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, digest, (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(t, 2, fake.ListReleasesCallCount())

	_, owner, repo, opts := fake.ListReleasesArgsForCall(1)
	require.Equal(t, "test", owner)
	require.Equal(t, "repo", repo)
	require.Equal(t, 2, opts.Page)
}

func TestSnapStopsPagingWhenTheReleaseIsMissing(t *testing.T) {
	fake := &githubfakes.FakeClient{}
	resp, err := notFound()
	fake.GetReleaseByTagReturns(nil, resp, err)
	fake.ListReleasesReturns([]*gogithub.RepositoryRelease{
		testRelease("v2.0.0", false),
	}, nil, nil)

	_, err = testDriver(fake).Snap()
	require.ErrorIs(t, err, ErrReleaseNotFound)
	// A short page means there is nothing else to read
	require.Equal(t, 1, fake.ListReleasesCallCount())
}

func TestSnapDoesNotFallBackOnOtherAPIErrors(t *testing.T) {
	fake := &githubfakes.FakeClient{}
	fake.GetReleaseByTagReturns(nil, &gogithub.Response{
		Response: &http.Response{StatusCode: http.StatusInternalServerError},
	}, errors.New("boom"))

	_, err := testDriver(fake).Snap()
	require.Error(t, err)
	require.Equal(t, 0, fake.ListReleasesCallCount())
}

func TestSnapHashesAssetsWithoutDigest(t *testing.T) {
	data := "artifact stored before github recorded digests"
	fake := &githubfakes.FakeClient{}
	fake.GetReleaseByTagReturns(
		testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "")), nil, nil,
	)
	fake.DownloadReleaseAssetReturns(io.NopCloser(strings.NewReader(data)), "", nil)

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, sha256Of(data), (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(t, 1, fake.DownloadReleaseAssetCallCount())

	_, owner, repo, assetID := fake.DownloadReleaseAssetArgsForCall(0)
	require.Equal(t, "test", owner)
	require.Equal(t, "repo", repo)
	require.Equal(t, int64(1), assetID)
}

func TestSnapHashesAssetsWithUnusableDigest(t *testing.T) {
	data := "artifact digested with something else"
	fake := &githubfakes.FakeClient{}
	fake.GetReleaseByTagReturns(
		testRelease("v1.0.0", false, testAsset(1, "artifact.zip", "sha512:abc")), nil, nil,
	)
	fake.DownloadReleaseAssetReturns(io.NopCloser(strings.NewReader(data)), "", nil)

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Equal(t, sha256Of(data), (*snap)["artifact.zip"].Checksum["sha256"])
	require.Equal(t, 1, fake.DownloadReleaseAssetCallCount())
}

func TestSnapSkipsIgnoredAndIncompleteAssets(t *testing.T) {
	uploading := testAsset(3, "uploading.zip", "sha256:"+sha256Of("uploading"))
	uploading.State = gogithub.Ptr("starter")

	fake := &githubfakes.FakeClient{}
	fake.GetReleaseByTagReturns(testRelease(
		"v1.0.0", false,
		testAsset(1, "artifact.zip", "sha256:"+sha256Of("artifact")),
		testAsset(2, "artifact.zip.sig", "sha256:"+sha256Of("signature")),
		uploading,
	), nil, nil)

	snap, err := testDriver(fake).Snap()
	require.NoError(t, err)
	require.Len(t, *snap, 1)
	require.Contains(t, *snap, "artifact.zip")
	require.Equal(t, 0, fake.DownloadReleaseAssetCallCount())
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
