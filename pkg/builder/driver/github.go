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
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/puerco/tejolote/pkg/run"
	"github.com/sirupsen/logrus"
)

type GitHubWorkflow struct {
	Organization string
	Repository   string
	RunID        int
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

type ghAPIResponseActor struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

type ghAPIResponseRun struct {
	ID              string             `json:"id"`
	HeadBranch      string             `json:"head_branch"`
	HeadSHA         string             `json:"head_sha"`
	Path            string             `json:"path"`
	RunNumber       int                `json:"run_number"`
	WorkFlowID      string             `json:"workflow_id"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
	LogsURL         string             `json:"logs_url"`
	Actor           ghAPIResponseActor `json:"actor"`
	TriggeringActor ghAPIResponseActor `json:"triggering_actor"`
}

const ghRunURL string = "https://api.github.com/repos/%s/%s/actions/runs/%d"

// RefreshRun queries the github API to get the latest data
func (ghw *GitHubWorkflow) RefreshRun(r *run.Run) error {
	// https://api.github.com/repos/distroless/static/actions/runs/2858064062
	res, err := gitHubAPIGetRequest(fmt.Sprintf(ghRunURL, ghw.Organization, ghw.Repository, ghw.RunID))
	if err != nil {
		return fmt.Errorf("querying github api: %w", err)
	}

	rawData, err := io.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		return fmt.Errorf("reading api response data: %w", err)
	}

	runData := &ghAPIResponseRun{}
	if err := json.Unmarshal(rawData, runData); err != nil {
		return fmt.Errorf("unmarshalling GitHub response: %w", err)
	}

	// FIXME: Assign data to the run

	return nil
}

// Perform an authenticated request to the GitHub api
func gitHubAPIGetRequest(url string) (*http.Response, error) {
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
