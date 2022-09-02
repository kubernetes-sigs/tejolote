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
