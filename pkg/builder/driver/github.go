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
	"strconv"
	"strings"
	"time"

	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"
	"github.com/sirupsen/logrus"

	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/github"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store"
)

const ghRunURL string = "https://api.github.com/repos/%s/%s/actions/runs/%d"

type GitHubWorkflow struct {
	Organization string
	Repository   string
	RunID        int
}

func parseGitHubURL(specURL string) (org, repo string, runID int64, err error) {
	u, err := url.Parse(specURL)
	if u.Scheme != "github" {
		return org, repo, runID, errors.New("URL is not a github URL")
	}
	if err != nil {
		return org, repo, runID, fmt.Errorf("parsing spec url: %w", err)
	}
	parts := strings.SplitN(u.Path, "/", 3)
	rID, err := strconv.Atoi(strings.TrimSuffix(parts[2], "/"))
	if err != nil {
		return org, repo, runID, fmt.Errorf("parsing run ID from URL: %w", err)
	}

	return u.Hostname(), parts[1], int64(rID), nil

}

func (ghw *GitHubWorkflow) GetRun(specURL string) (*run.Run, error) {
	r := &run.Run{
		SpecURL:   specURL,
		IsSuccess: false,
		Steps:     []run.Step{},
		Artifacts: []run.Artifact{},
		StartTime: time.Time{},
		EndTime:   time.Time{},
	}
	if err := ghw.RefreshRun(r); err != nil {
		return nil, fmt.Errorf("doing initial refresh of run data: %w", err)
	}
	return r, nil
}

// RefreshRun queries the github API to get the latest data
func (ghw *GitHubWorkflow) RefreshRun(r *run.Run) error {
	// https://api.github.com/repos/distroless/static/actions/runs/2858064062
	// https://api.github.com/repos/distroless/static/actions/runs/7492361110 (failure)
	org, repo, id, err := parseGitHubURL(r.SpecURL)
	if err != nil {
		return fmt.Errorf("parsing spec url: %w", err)
	}
	ghw.Organization = org
	ghw.Repository = repo
	ghw.RunID = int(id)

	res, err := github.APIGetRequest(fmt.Sprintf(ghRunURL, ghw.Organization, ghw.Repository, ghw.RunID))
	if err != nil {
		return fmt.Errorf("querying github api: %w", err)
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("got https error %d from github API", res.StatusCode)
	}

	rawData, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return fmt.Errorf("reading api response data: %w", err)
	}

	logrus.Debugf("Rawdata: %s", string(rawData))

	runData := &github.Run{}
	if err := json.Unmarshal(rawData, runData); err != nil {
		return fmt.Errorf("unmarshalling GitHub response: %w", err)
	}

	if runData.Status == "completed" {
		r.IsRunning = false
	}

	switch runData.Conclusion {
	case "failure", "cancelled":
		r.IsSuccess = false
	case "success":
		r.IsSuccess = true
	}

	r.SystemData = runData

	// TODO: Consider pulling the job data if specified and the workflow yaml.
	// Using those we can populate the entry point better to the job, the label of
	// the runner

	return nil
}

// BuildPredicate builds a predicate from the run data
func (ghw *GitHubWorkflow) BuildPredicate(
	r *run.Run, draft *attestation.SLSAPredicate,
) (predicate *attestation.SLSAPredicate, err error) {
	type githubEnvironment struct {
		// The architecture of the runner.
		Arch string            `json:"arch"`
		Env  map[string]string `json:"env"`
		// The context values that were referenced in the workflow definition.
		// Secrets are set to the empty string.
		Context struct {
			GitHub map[string]string `json:"github"`
			Runner map[string]string `json:"runner"`
		} `json:"context"`
	}
	org, repo, runID, err := parseGitHubURL(r.SpecURL)
	if err != nil {
		return nil, fmt.Errorf("parsing run spec URL: %w", nil)
	}
	if draft == nil {
		pred := attestation.NewSLSAPredicate()
		predicate = &pred
	} else {
		predicate = draft
	}
	(*predicate).Builder.ID = "https://github.com/Attestations/GitHubHostedActions@v1"
	(*predicate).BuildType = "https://github.com/Attestations/GitHubActionsWorkflow@v1"
	(*predicate).Invocation.ConfigSource.Digest = slsa.DigestSet{
		"sha1": r.SystemData.(*github.Run).HeadSHA,
	}
	(*predicate).Invocation.ConfigSource.EntryPoint = r.SystemData.(*github.Run).Path
	(*predicate).Invocation.ConfigSource.URI = fmt.Sprintf(
		"git+https://github.com/%s/%s.git", org, repo,
	)
	// TODO: I think we need to checkout the file from git to fill
	(*predicate).Invocation.Environment = githubEnvironment{
		Arch: "",
		Env:  map[string]string{},
		Context: struct {
			GitHub map[string]string `json:"github"`
			Runner map[string]string `json:"runner"`
		}{
			GitHub: map[string]string{
				"run_id": fmt.Sprintf("%d", runID),
			},
		},
	}
	return predicate, nil
}

// ArtifactStores returns the native artifact store of github actions
func (ghw *GitHubWorkflow) ArtifactStores() []store.Store {
	d, err := store.New(
		fmt.Sprintf(
			"actions://%s/%s/%d",
			ghw.Organization, ghw.Repository, ghw.RunID,
		),
	)
	if err != nil {
		logrus.Error(err)
		return []store.Store{}
	}
	return []store.Store{d}
}
