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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		if err := github.Download(a.URL, f); err != nil {
			return nil, fmt.Errorf(
				"downloading artifact from %s: %w", a.URL, err,
			)
		}
		shaVal, err := hash.SHA256ForFile(f.Name())
		if err != nil {
			return nil, fmt.Errorf("hashing file: %w", err)
		}
		ret = append(ret, run.Artifact{
			Path: runURL + "/" + a.Name,
			Checksum: map[string]string{
				string(intoto.AlgorithmSHA256): shaVal,
			},
			Time: a.UpdatedAt,
		})
	}
	logrus.Infof("%d artifacts collected from run %d", len(ret), a.RunID)
	return ret, nil
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
