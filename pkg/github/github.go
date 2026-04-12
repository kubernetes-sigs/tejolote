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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v84/github"
	"github.com/sirupsen/logrus"
	khttp "sigs.k8s.io/release-utils/http"
)

// TokenScopes returns the scopes of token in the eviroment
func TokenScopes() ([]string, error) {
	res, err := APIGetRequest("https://api.github.com/repos/github/docs")
	if err != nil {
		return nil, fmt.Errorf("making request to API: %w", err)
	}
	defer res.Body.Close()

	header := res.Header.Get("X-Oauth-Scopes")
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

func APIGetRequest(url string) (*http.Response, error) {
	logrus.Debugf("GitHubAPI[GET]: %s", url)
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
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
		return res, fmt.Errorf("executing http request to GitHub API: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"http error %d making request to GitHub API", res.StatusCode,
		)
	}
	return res, nil
}

// GetRunJobs fetches the jobs for a given workflow run from the GitHub API.
func GetRunJobs(org, repo string, runID int64) ([]*gogithub.WorkflowJob, error) {
	u := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/actions/runs/%d/jobs",
		org, repo, runID,
	)
	res, err := APIGetRequest(u)
	if err != nil {
		return nil, fmt.Errorf("querying jobs API: %w", err)
	}
	defer res.Body.Close()

	rawData, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading jobs response: %w", err)
	}

	var jobsResp gogithub.Jobs
	if err := json.Unmarshal(rawData, &jobsResp); err != nil {
		return nil, fmt.Errorf("unmarshalling jobs response: %w", err)
	}

	return jobsResp.Jobs, nil
}

func Download(url string, f io.Writer) error {
	agent := NewAgent()
	return agent.GetToWriter(f, url)
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
