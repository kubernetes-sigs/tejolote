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

// Artifact is the artifact structure returned by the API
type Artifact struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Size      int       `json:"size_in_bytes"`
	URL       string    `json:"archive_download_url"`
	Expired   bool      `json:"expired"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Run struct {
	ID              int64  `json:"id"`
	Status          string `json:"status"`
	Conclusion      string `json:"conclusion"`
	HeadBranch      string `json:"head_branch"`
	HeadSHA         string `json:"head_sha"`
	Path            string `json:"path"`
	RunNumber       int64  `json:"run_number"`
	WorkFlowID      int64  `json:"workflow_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
	LogsURL         string `json:"logs_url"`
	Actor           Actor  `json:"actor"`
	TriggeringActor Actor  `json:"triggering_actor"`
}

type Actor struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}
