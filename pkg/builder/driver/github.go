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
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v84/github"
	intoto "github.com/in-toto/attestation/go/v1"
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

	// workflow caches the parsed workflow YAML data to avoid
	// repeated fetches when building predicates or discovering jobs.
	workflow *github.WorkflowData
}

func parseGitHubURL(specURL string) (org, repo string, runID int64, err error) {
	u, err := url.Parse(specURL)
	if u.Scheme != GITHUB {
		return org, repo, runID, errors.New("URL is not a github URL")
	}
	if err != nil {
		return org, repo, runID, fmt.Errorf("parsing spec url: %w", err)
	}
	parts := strings.SplitN(u.Path, "/", 3)
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid run URI")
	}
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

	if res.StatusCode != http.StatusOK {
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

	r.BuildPoint = &intoto.ResourceDescriptor{
		Uri: fmt.Sprintf("git+ssh://github.com/%s/%s@%s", org, repo, runData.HeadSHA),
		Digest: map[string]string{
			"sha1": runData.HeadSHA,
		},
	}

	// TODO: Consider pulling the job data if specified and the workflow yaml.
	// Using those we can populate the entry point better to the job, the label of
	// the runner

	return nil
}

// GetWorkflow returns the parsed workflow YAML data, fetching and caching
// it on first call. Requires that RefreshRun has been called at least once
// so that Organization, Repository and the run's SystemData are populated.
func (ghw *GitHubWorkflow) GetWorkflow(r *run.Run) (*github.WorkflowData, error) {
	if ghw.workflow != nil {
		return ghw.workflow, nil
	}

	ghrun, ok := r.SystemData.(*github.Run)
	if !ok {
		return nil, fmt.Errorf("run system data is not a GitHub run")
	}

	wf, err := github.FetchWorkflow(
		ghw.Organization, ghw.Repository, ghrun.Path, ghrun.HeadSHA,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching workflow: %w", err)
	}

	ghw.workflow = wf
	return wf, nil
}

// GetRunJobs fetches the jobs for this workflow run from the GitHub API.
func (ghw *GitHubWorkflow) GetRunJobs() ([]*gogithub.WorkflowJob, error) {
	return github.GetRunJobs(
		ghw.Organization, ghw.Repository, int64(ghw.RunID),
	)
}

// BuildPredicate builds a predicate from the run data
func (ghw *GitHubWorkflow) BuildPredicate(
	r *run.Run, draft attestation.Predicate,
) (predicate attestation.Predicate, err error) {
	org, repo, runID, err := parseGitHubURL(r.SpecURL)
	if err != nil {
		return nil, fmt.Errorf("parsing run spec URL: %w", nil)
	}
	repo = strings.TrimSuffix(repo, ".git")
	if draft == nil {
		pred := attestation.NewSLSAPredicate()
		predicate = pred
	} else {
		predicate = draft
	}

	predicate.SetBuilderID("https://github.com/Attestations/GitHubHostedActions@v1")

	predicate.SetBuilderType("https://actions.github.io/buildtypes/workflow/v1")

	// Set the older builder type for SLSA 0.2:
	if predicate.Type() == "https://slsa.dev/provenance/v0.2" {
		predicate.SetBuilderType("https://github.com/Attestations/GitHubActionsWorkflow@v1")
	}

	confsource := &intoto.ResourceDescriptor{
		Uri:    fmt.Sprintf("git+https://github.com/%s/%s", org, repo),
		Digest: map[string]string{},
	}

	var event, repoId, ownerId string
	if ghrun, ok := r.SystemData.(*github.Run); ok {
		confsource.Digest["sha1"] = ghrun.HeadSHA
		confsource.Digest["gitCommit"] = ghrun.HeadSHA
		predicate.SetBuilderID(fmt.Sprintf("https://github.com/%s/%s/%s@%s", org, repo, ghrun.Path, ghrun.HeadSHA))
		predicate.SetInvocationID(fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d/attempts/%d", org, repo, runID, ghrun.RunAttempt))
		predicate.SetEntryPoint(ghrun.Path)
		predicate.SetStartedOn(ghrun.CreatedAt)
		predicate.SetFinishedOn(ghrun.UpdatedAt)
		event = ghrun.Event
		repoId = fmt.Sprintf("%d", ghrun.Repository.ID)
		ownerId = fmt.Sprintf("%d", ghrun.Repository.Owner.ID)

		predicate.AddExternalParameter(
			"workflow", map[string]any{
				"path":       ghrun.Path,
				"repository": fmt.Sprintf("https://github.com/%s/%s", org, repo),
			},
		)

		// Fetch the workflow YAML (cached) and compute effective inputs
		wf, err := ghw.GetWorkflow(r)
		if err != nil {
			return nil, fmt.Errorf("fetching workflow: %w", err)
		}

		definedInputs := wf.Inputs()
		if len(definedInputs) > 0 {
			effective := github.EffectiveInputs(definedInputs, ghrun.Inputs)
			for k, v := range effective {
				predicate.AddExternalParameter(k, v)
			}
		}
	}

	predicate.SetConfigSource(confsource)

	predicate.SetInternalParameters(
		map[string]any{
			"github": map[string]any{
				"event_name":          event,
				"repository_id":       repoId,
				"repository_owner_id": ownerId,
				"runner_environment":  "github-hosted",
			},
		},
	)

	// Compat with the old
	if predicate.Type() == "https://slsa.dev/provenance/v0.2" {
		predicate.SetInternalParameters(
			map[string]any{
				"arch": "",
				"env":  map[string]string{},
				"context": map[string]any{
					"github": map[string]string{
						"run_id": fmt.Sprintf("%d", runID),
					},
					"runner": map[string]string{},
				},
			},
		)
	}
	return predicate, nil
}

// AreJobsCompleted checks whether the specified jobs (by name) have all
// completed. If jobNames is empty, all jobs in the run are checked except
// the one matching excludeJob (useful for excluding the attester's own job).
// Job name matching is prefix-based to handle reusable workflow jobs whose
// API names are formatted as "caller_job / inner_job".
func (ghw *GitHubWorkflow) AreJobsCompleted(jobNames []string, excludeJob string) (bool, error) {
	jobs, err := ghw.GetRunJobs()
	if err != nil {
		return false, fmt.Errorf("fetching run jobs: %w", err)
	}

	for _, job := range jobs {
		name := job.GetName()
		status := job.GetStatus()

		// Skip the excluded job (our own attester job)
		if excludeJob != "" && matchJobName(name, excludeJob) {
			logrus.Debugf("Skipping excluded job %q", name)
			continue
		}

		// If specific jobs were requested, only check those
		if len(jobNames) > 0 && !matchesAnyJobName(name, jobNames) {
			continue
		}

		if status != "completed" {
			logrus.Infof("Job %q status: %s — still running", name, status)
			return false, nil
		}

		logrus.Debugf("Job %q completed with conclusion: %s", name, job.GetConclusion())
	}

	return true, nil
}

// matchJobName checks if an API job name matches a YAML job key.
// GitHub Actions formats reusable workflow job names as "caller_key / inner_job",
// so we match if the API name equals the key or starts with "key / ".
func matchJobName(apiName, yamlKey string) bool {
	return apiName == yamlKey || strings.HasPrefix(apiName, yamlKey+" / ")
}

// matchesAnyJobName checks if an API job name matches any of the provided names.
func matchesAnyJobName(apiName string, names []string) bool {
	for _, n := range names {
		if matchJobName(apiName, n) {
			return true
		}
	}
	return false
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
