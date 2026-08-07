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

import "time"

type Run struct {
	ID              int64             `json:"id"`
	Status          string            `json:"status"`
	Conclusion      string            `json:"conclusion"`
	HeadBranch      string            `json:"head_branch"`
	HeadSHA         string            `json:"head_sha"`
	Path            string            `json:"path"`
	RunNumber       int64             `json:"run_number"`
	WorkFlowID      int64             `json:"workflow_id"`
	RunAttempt      int64             `json:"run_attempt"`
	CreatedAt       *time.Time        `json:"created_at"`
	UpdatedAt       *time.Time        `json:"updated_at"`
	LogsURL         string            `json:"logs_url"`
	Event           string            `json:"event"`
	Inputs          map[string]string `json:"inputs"`
	Actor           Actor             `json:"actor"`
	TriggeringActor Actor             `json:"triggering_actor"`
	Repository      RunRepository     `json:"repository"`
}

type Actor struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

type RunRepository struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	FullName string       `json:"full_name"`
	Owner    RunRepoOwner `json:"owner"`
}

type RunRepoOwner struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}
