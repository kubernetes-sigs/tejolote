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
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	intoto "github.com/in-toto/attestation/go/v1"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/hash"
	"sigs.k8s.io/tejolote/pkg/github"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

const actionsArtifactsURL = "https://api.github.com/repos/%s/%s/actions/runs/%d/artifacts"

// const actionsArtifactsURL =    "https://api.github.com/repos/%s/%s/actions/artifacts/%d"

type Actions struct {
	Organization string
	Repository   string
	RunID        int

	// Expand controls how artifacts are hashed. When true (the default) each
	// artifact zip is unpacked and every contained file becomes its own subject,
	// hashed by content. When false, the downloaded artifact archive is hashed
	// as a single subject.
	Expand bool

	// Filter, when non-empty, is a glob (path.Match syntax) matched against
	// artifact names; only matching artifacts are collected.
	Filter string
}

var ErrNoWorkflowToken = errors.New("token does not have workflow scope")

func NewActions(specURL string) (*Actions, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing SpecURL %s: %w", specURL, err)
	}
	if u.Scheme != "actions" {
		return nil, errors.New("spec url is not an actions run")
	}
	repo, runids, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	runid, err := strconv.Atoi(runids)
	if err != nil {
		return nil, fmt.Errorf("unable to read runid from %s", u.Path)
	}

	a := &Actions{
		Organization: u.Hostname(),
		Repository:   repo,
		RunID:        runid,
		Expand:       true,
	}
	return a, nil
}

// readArtifacts gets the artiofacts from the run
func (a *Actions) readArtifacts() ([]run.Artifact, error) {
	runURL := fmt.Sprintf(
		actionsArtifactsURL,
		a.Organization, a.Repository, a.RunID,
	)

	res, err := github.APIGetRequest(runURL)
	if err != nil {
		return nil, fmt.Errorf("querying GitHub api for artifacts: %w", err)
	}
	rawData, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading api response data: %w", err)
	}

	artifacts := struct {
		Artifacts []github.Artifact `json:"artifacts"`
	}{
		Artifacts: []github.Artifact{},
	}

	if err := json.Unmarshal(rawData, &artifacts); err != nil {
		return nil, fmt.Errorf("unmarshalling GitHub response: %w", err)
	}

	// Filter the artifacts by name (glob) if a filter is configured, so we only
	// download and attest the ones we care about.
	selected := make([]github.Artifact, 0, len(artifacts.Artifacts))
	for _, art := range artifacts.Artifacts {
		if a.Filter != "" {
			match, err := path.Match(a.Filter, art.Name)
			if err != nil {
				return nil, fmt.Errorf("invalid artifacts filter %q: %w", a.Filter, err)
			}
			if !match {
				logrus.Debugf("artifact %q does not match filter %q, skipping", art.Name, a.Filter)
				continue
			}
		}
		selected = append(selected, art)
	}

	// Download the selected artifacts to hash them.
	tmpdir, err := os.MkdirTemp("", "artifacts-")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	// Create files and writers for parallel download
	urls := make([]string, len(selected))
	files := make([]*os.File, len(selected))
	writers := make([]io.Writer, len(selected))
	for i, art := range selected {
		f, err := os.Create(filepath.Join(tmpdir, art.Name))
		if err != nil {
			return nil, fmt.Errorf("creating artifact file: %w", err)
		}
		defer f.Close()
		urls[i] = art.URL
		files[i] = f
		writers[i] = f
	}

	// Download all artifacts in parallel
	agent := github.NewAgent()
	errs := agent.GetToWriterGroup(writers, urls)
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("downloading artifacts: %w", err)
	}

	// Each downloaded artifact is a ZIP archive (the GitHub Actions artifact
	// download API always returns a zip wrapping the uploaded files). When Expand
	// is set we unpack each one and emit a subject per contained file, hashed by
	// its content and named by its path within the zip. Otherwise we hash the
	// archive itself as a single subject, keeping the prior subject name (the
	// artifacts API URL joined with the artifact name) for compatibility.
	ret := make([]run.Artifact, 0, len(selected))
	for i, art := range selected {
		if a.Expand {
			subjects, err := hashArtifactZip(files[i].Name(), art.Name, art.URL, art.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("hashing artifact %q: %w", art.Name, err)
			}
			ret = append(ret, subjects...)
			continue
		}

		shaVal, err := hash.SHA256ForFile(files[i].Name())
		if err != nil {
			return nil, fmt.Errorf("hashing artifact %q: %w", art.Name, err)
		}
		ret = append(ret, run.Artifact{
			Path:     runURL + "/" + art.Name,
			URL:      art.URL,
			Checksum: map[string]string{string(intoto.AlgorithmSHA256): shaVal},
			Time:     art.UpdatedAt,
		})
	}
	logrus.Infof("collected %d subjects from %d artifacts in run %d", len(ret), len(selected), a.RunID)
	return ret, nil
}

// hashArtifactZip unpacks a downloaded GitHub Actions artifact (always a zip)
// and returns one subject per contained file, each hashed by its content. Each
// subject is named "<artifactName>/<path within the zip>" so files sharing a
// name across different artifacts (e.g. checksums.txt) stay distinct, and its
// uri points at the specific file inside the archive. If the payload is not a
// valid zip it falls back to hashing the raw blob as a single subject.
func hashArtifactZip(zipPath, artifactName, artifactURL string, updated time.Time) ([]run.Artifact, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		// Not a zip (should not happen for Actions artifacts): hash the blob.
		logrus.Warnf("artifact %q is not a zip archive, hashing raw blob: %v", artifactName, err)
		shaVal, herr := hash.SHA256ForFile(zipPath)
		if herr != nil {
			return nil, herr
		}
		return []run.Artifact{{
			Path:     artifactName,
			URL:      artifactURL,
			Checksum: map[string]string{string(intoto.AlgorithmSHA256): shaVal},
			Time:     updated,
		}}, nil
	}
	defer zr.Close()

	subjects := make([]run.Artifact, 0, len(zr.File))
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		shaVal, herr := sha256ZipEntry(zf)
		if herr != nil {
			return nil, fmt.Errorf("hashing %s: %w", zf.Name, herr)
		}
		// Drop the leading slash so a hostile entry name (eg "../../x") cannot
		// escape the artifact-name prefix
		entry := strings.TrimPrefix(path.Clean("/"+zf.Name), "/")
		subjects = append(subjects, run.Artifact{
			Path:     artifactName + "/" + entry,
			URL:      zipEntryURI(artifactURL, entry),
			Checksum: map[string]string{string(intoto.AlgorithmSHA256): shaVal},
			Time:     updated,
		})
	}
	return subjects, nil
}

// zipEntryURI points an artifact's download URL at a specific file inside the
// zip using the URL fragment, eg .../artifacts/42/zip#bin/tejolote. The entry
// path is encoded so names with special characters are still a valid URI
func zipEntryURI(artifactURL, entry string) string {
	u, err := url.Parse(artifactURL)
	if err != nil {
		return artifactURL + "#" + entry
	}
	u.Fragment = entry
	return u.String()
}

// maxZipEntrySize caps how many bytes tejolote will read and hash from a single
// file inside a GitHub Actions artifact zip. Real artifacts are far smaller and
// a larger declared size may be a decompression bomb
const maxZipEntrySize = 10 << 30 // 10 GiB

// sha256ZipEntry returns the hex-encoded SHA256 of a zip entry's contents.
func sha256ZipEntry(zf *zip.File) (string, error) {
	rc, err := zf.Open()
	if err != nil {
		return "", fmt.Errorf("opening zip entry: %w", err)
	}
	defer rc.Close()

	// Reject entries declaring a size larger than maxZipEntrySize and then only
	// copy up to those bytes (gosec G110).
	size := zf.UncompressedSize64
	if size > maxZipEntrySize {
		return "", fmt.Errorf(
			"zip entry %q declares uncompressed size %d bytes, exceeding the %d byte limit",
			zf.Name, size, uint64(maxZipEntrySize),
		)
	}

	h := sha256.New()
	if _, err := io.CopyN(h, rc, int64(size)); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading zip entry: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Snap returns a snapshot of the current state
func (a *Actions) Snap() (*snapshot.Snapshot, error) {
	artifacts, err := a.readArtifacts()
	if err != nil {
		return nil, fmt.Errorf("collecting artifacts: %w", err)
	}
	snap := snapshot.Snapshot{}
	for _, a := range artifacts {
		snap[a.Path] = a
	}
	return &snap, nil
}
