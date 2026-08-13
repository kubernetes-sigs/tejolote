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

package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gogithub "github.com/google/go-github/v90/github"
	"github.com/sirupsen/logrus"
	khttp "sigs.k8s.io/release-utils/http"
)

// itemsPerPage is the page size used when reading paginated API listings.
const itemsPerPage = 100

// defaultClient returns the client shared by this package, built on first use.
// Note that the GITHUB_TOKEN value is captured on first use and reused for the
// lifetime of the process, later changes to the environment have no effect.
var defaultClient = sync.OnceValues(NewClient)

// TokenScopes returns the scopes of token in the eviroment
func TokenScopes() ([]string, error) {
	client, err := defaultClient()
	if err != nil {
		return nil, fmt.Errorf("creating github client: %w", err)
	}

	// Any authenticated request echoes the token scopes back in a header
	_, res, err := client.Repositories.Get(context.Background(), "github", "docs")
	if err != nil {
		return nil, fmt.Errorf("making request to API: %w", err)
	}

	header := res.Header.Get("X-Oauth-Scopes")
	if header == "" {
		return []string{}, nil
	}

	scopes := strings.Split(header, ", ")
	logrus.Debugf("GitHub Token scopes: %+v", scopes)
	return scopes, nil
}

// TokenHas returns a bool if the token in use has the scope passed
func TokenHas(scope string) (bool, error) {
	scopes, err := TokenScopes()
	if err != nil {
		return false, fmt.Errorf("reading scopes: %w", err)
	}
	for _, s := range scopes {
		if s == scope {
			return true, nil
		}
	}
	return false, nil
}

// GetRun fetches the data of a workflow run.
//
// The response is read into tejolote's own run type instead of go-github's:
// the provenance predicate is built from fields the API returns but the
// go-github type does not model, such as the workflow_dispatch inputs.
func GetRun(org, repo string, runID int64) (*Run, error) {
	client, err := defaultClient()
	if err != nil {
		return nil, fmt.Errorf("creating github client: %w", err)
	}

	req, err := client.NewRequest(
		context.Background(), http.MethodGet,
		fmt.Sprintf("repos/%s/%s/actions/runs/%d", org, repo, runID), nil,
	)
	if err != nil {
		return nil, fmt.Errorf("building run request: %w", err)
	}

	runData := &Run{}
	if _, err := client.Do(req, runData); err != nil {
		return nil, fmt.Errorf("querying run API: %w", err)
	}
	return runData, nil
}

// GetRunJobs fetches the jobs for a given workflow run from the GitHub API.
func GetRunJobs(org, repo string, runID int64) ([]*gogithub.WorkflowJob, error) {
	client, err := defaultClient()
	if err != nil {
		return nil, fmt.Errorf("creating github client: %w", err)
	}

	ctx := context.Background()
	opts := &gogithub.ListWorkflowJobsOptions{
		ListOptions: gogithub.ListOptions{PerPage: itemsPerPage},
	}

	jobs := []*gogithub.WorkflowJob{}
	for {
		page, res, err := client.Actions.ListWorkflowJobs(ctx, org, repo, runID, opts)
		if err != nil {
			return nil, fmt.Errorf("querying jobs API: %w", err)
		}

		jobs = append(jobs, page.Jobs...)
		if res.NextPage == 0 {
			break
		}
		opts.Page = res.NextPage
	}

	return jobs, nil
}

// ListRunArtifacts returns the artifacts a workflow run stored in the GitHub
// Actions artifact store.
func ListRunArtifacts(org, repo string, runID int64) ([]*gogithub.Artifact, error) {
	client, err := defaultClient()
	if err != nil {
		return nil, fmt.Errorf("creating github client: %w", err)
	}

	ctx := context.Background()
	opts := &gogithub.ListOptions{PerPage: itemsPerPage}

	artifacts := []*gogithub.Artifact{}
	for {
		page, res, err := client.Actions.ListWorkflowRunArtifacts(ctx, org, repo, runID, opts)
		if err != nil {
			return nil, fmt.Errorf("querying artifacts API: %w", err)
		}

		artifacts = append(artifacts, page.Artifacts...)
		if res.NextPage == 0 {
			break
		}
		opts.Page = res.NextPage
	}

	return artifacts, nil
}

// GetCurrentJob returns the WorkflowJob for the job currently executing inside a
// GitHub Actions runner.
// It identifies the job by matching the given runner name (from the RUNNER_NAME
// environment variable) against the run's jobs, restricted to those that are still
// in progress. This uniquely identifies the caller even when the job definition
// sets a custom name: GITHUB_JOB only exposes the job key, while the jobs API
// reports the display name, so the two differ in that case. It returns (nil, nil)
// when there is no unique match (eg when the RUNNER_NAME is unset or more than
// one job in progress shares the runner name).
func GetCurrentJob(org, repo string, runID int64, runnerName string) (*gogithub.WorkflowJob, error) {
	if runnerName == "" {
		return nil, nil
	}

	jobs, err := GetRunJobs(org, repo, runID)
	if err != nil {
		return nil, fmt.Errorf("fetching run jobs: %w", err)
	}

	var match *gogithub.WorkflowJob
	for _, job := range jobs {
		if job.GetStatus() == "in_progress" && job.GetRunnerName() == runnerName {
			if match != nil {
				// Ambiguous: more than one in-progress job on this runner name.
				return nil, nil
			}
			match = job
		}
	}
	return match, nil
}

func Download(url string, f io.Writer) error {
	agent := NewAgent()
	return agent.GetToWriter(f, url)
}

// NewClient returns a GitHub API client authenticated with the token in the
// environment. Without a token the client still works but GitHub throttles
// unauthenticated requests much more aggressively.
func NewClient() (*gogithub.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		logrus.Warn("making unauthenticated requests to github")
		return gogithub.NewClient()
	}
	return gogithub.NewClient(gogithub.WithAuthToken(token))
}

// NewAgent returns a new khttp.Agent configured with GitHub authentication.
func NewAgent() *khttp.Agent {
	agent := khttp.NewAgent().WithTimeout(5 * time.Minute).WithFailOnHTTPError(true)
	agent.SetImplementation(&githubAgentImpl{})
	return agent
}

// githubAgentImpl injects the GitHub token into requests.
type githubAgentImpl struct{}

func (g *githubAgentImpl) SendGetRequest(client *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	setGitHubAuth(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing http request: %w", err)
	}
	return resp, nil
}

func (g *githubAgentImpl) SendPostRequest(client *http.Client, u string, postData []byte, contentType string) (*http.Response, error) {
	return nil, errors.New("POST not supported for GitHub agent")
}

func (g *githubAgentImpl) SendHeadRequest(client *http.Client, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	setGitHubAuth(req)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing http request: %w", err)
	}
	return resp, nil
}

func setGitHubAuth(req *http.Request) {
	if os.Getenv("GITHUB_TOKEN") != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN")))
	} else {
		logrus.Warn("making unauthenticated request to github")
	}
}
