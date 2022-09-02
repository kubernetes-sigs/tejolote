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

package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/hash"

	"github.com/puerco/tejolote/pkg/run"
	"github.com/puerco/tejolote/pkg/store/snapshot"
)

const actionsArtifactsURL = "https://api.github.com/repos/%s/%s/actions/runs/%d/artifacts"

type Actions struct {
	Organization string
	Repository   string
	RunID        int
}

func NewActions(specURL string) (*Actions, error) {
	// TODO: We need to check the scopes of the token to ensure we
	// have the actions scope:
	// https://docs.github.com/rest/reference/actions#download-an-artifact
	// Issue a request to get the user identity and check this header
	// in the response:
	// x-oauth-scopes: read:discussion, read:gpg_key, read:org, read:public_key, read:user, repo, workflow, write:packages
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
	}
	return a, nil
}

type actionsArtifactAPI struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Size      int       `json:"size_in_bytes"`
	URL       string    `json:"archive_download_url"`
	Expired   bool      `json:"expired"`
	UpdatedAt time.Time `json:"updated_at"`
}

// readArtifacts gets the artiofacts from the run
func (a *Actions) readArtifacts() ([]run.Artifact, error) {
	runUrl := fmt.Sprintf(
		actionsArtifactsURL,
		a.Organization, a.Repository, a.RunID,
	)

	res, err := gitHubAPIGetRequest(runUrl)
	if err != nil {
		return nil, fmt.Errorf("querying GitHub api for artifacts: %w", err)
	}
	rawData, err := io.ReadAll(res.Body)
	logrus.Info(string(rawData))
	defer res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading api response data: %w", err)
	}

	artifacts := struct {
		Artifacts []actionsArtifactAPI `json:"artifacts"`
	}{
		Artifacts: []actionsArtifactAPI{},
	}

	if err := json.Unmarshal(rawData, &artifacts); err != nil {
		return nil, fmt.Errorf("unmarshalling GitHub response: %w", err)
	}

	// Now we need to download the artifacts to hash them
	tmpdir, err := os.MkdirTemp("", "artifacts-")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	ret := []run.Artifact{}

	for _, a := range artifacts.Artifacts {
		f, err := os.Create(filepath.Join(tmpdir, a.Name))
		if err != nil {
			return nil, fmt.Errorf("creating artifact file: %w", err)
		}
		defer f.Close()
		if err := httpDownload(a.URL, f); err != nil {
			return nil, fmt.Errorf(
				"downloading artifact from %s: %w", a.URL, err,
			)
		}
		shaVal, err := hash.SHA256ForFile(f.Name())
		if err != nil {
			return nil, fmt.Errorf("hashing file: %w", err)
		}
		ret = append(ret, run.Artifact{
			Path: runUrl + "/" + a.Name,
			Checksum: map[string]string{
				"SHA256": shaVal,
			},
			Time: a.UpdatedAt,
		})
	}
	logrus.Infof("%d artifacts collected from run %d", len(ret), a.RunID)
	return ret, nil
}

func httpDownload(url string, f io.Writer) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error when downloading: %s", resp.Status)
	}

	// Writer the body to file
	numBytes, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("writing http response to disk: %w", err)
	}
	logrus.Infof("%d MB downloaded from %s", (numBytes / 1024 / 1024), url)
	return nil
}

// Perform an authenticated request to the GitHub api
// NOTE: We should move this function to a common location
// to share with the github builder
func gitHubAPIGetRequest(url string) (*http.Response, error) {
	logrus.Infof("GHAPI[GET]: %s", url)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if os.Getenv("GITHUB_TOKEN") != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN")))
	} else {
		logrus.Warn("making unauthenticated request to github")
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing http requet to GitHub API: %w", err)
	}
	return res, nil
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
