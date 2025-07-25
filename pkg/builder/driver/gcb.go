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

/*
  This is not yet implemented, but probably we should use the full
  URL as handled internally in the GCP API, eg:
  projects/648026197307/locations/global/builds/ba067a55-6090-4080-bc1a-6d1ff944fd60

*/

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	v1 "github.com/in-toto/attestation/go/v1"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/cloudbuild/v1"
	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store"
)

type GCB struct {
	ProjectID string
	BuildID   string
}

func NewGCB(specURL string) (*GCB, error) {
	project, build, err := parseGCBURL(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing gcb url: %w", err)
	}

	return &GCB{
		ProjectID: project,
		BuildID:   build,
	}, nil
}

func (gcb *GCB) GetRun(specURL string) (*run.Run, error) {
	r := &run.Run{
		SpecURL:   specURL,
		IsSuccess: false,
		Steps:     []run.Step{},
		Artifacts: []run.Artifact{},
		StartTime: time.Time{},
		EndTime:   time.Time{},
	}
	if err := gcb.RefreshRun(r); err != nil {
		return nil, fmt.Errorf("doing initial refresh of run data: %w", err)
	}
	return r, nil

	// TODO: review this
	//nolint: gocritic
	/*
		req, err := cloudbuildService.Projects.Builds.List(project).Filter(tagsFilter).PageSize(50).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to listing the builds: %w", err)
		}
	*/
}

func parseGCBURL(gcbURL string) (host, path string, err error) {
	// Fetch the required data to get the build from the URL
	u, err := url.Parse(gcbURL)
	if err != nil {
		return "", "", fmt.Errorf("parsing GCB spec URL: %w", err)
	}
	return u.Hostname(), strings.TrimPrefix(u.Path, "/"), nil
}

// RefreshRun queries the API from the build system and
// updates the run metadata.
func (gcb *GCB) RefreshRun(r *run.Run) error {
	project, buildID, err := parseGCBURL(r.SpecURL)
	if err != nil {
		return fmt.Errorf("parsing GCB spec URL: %w", err)
	}

	ctx := context.Background()
	cloudbuildService, err := cloudbuild.NewService(ctx)
	if err != nil {
		return fmt.Errorf("creating cloudbuild client: %w", err)
	}
	build, err := cloudbuildService.Projects.Builds.Get(project, buildID).Do()
	if err != nil {
		return fmt.Errorf("getting build %s from GCB: %w", buildID, err)
	}
	logrus.Debugf("%+v", build)
	r.Params = []string{}
	for k, v := range build.Substitutions {
		r.Params = append(r.Params, fmt.Sprintf("%s=%s", k, v))
	}

	for i, s := range build.Steps {
		logrus.Debugf("Step #%d %+v", i, s)
		if len(r.Steps) <= i {
			r.Steps = append(r.Steps, run.Step{
				Params:      []string{},
				Environment: map[string]string{},
			})
		}
		//
		r.Steps[i].Image = s.Name
		r.Steps[i].Params = s.Args
		if s.Timing != nil {
			if s.Timing.StartTime == "" {
				stime, err := time.Parse(time.RFC3339Nano, s.Timing.StartTime)
				if s.Timing.EndTime != "" && err != nil {
					return fmt.Errorf("parsing step start time")
				}
				r.Steps[i].StartTime = stime
			} else {
				etime, err := time.Parse(time.RFC3339Nano, s.Timing.EndTime)
				if s.Timing.EndTime != "" && err != nil {
					return fmt.Errorf("parsing step end time")
				}
				r.Steps[i].EndTime = etime
			}

			if s.Timing.EndTime == "" {
				r.Steps[i].EndTime = time.Time{}
			} else {
				etime, err := time.Parse(time.RFC3339Nano, s.Timing.EndTime)
				if s.Timing.EndTime != "" && err != nil {
					return fmt.Errorf("parsing step endtime")
				}
				r.Steps[i].EndTime = etime
			}
		}
	}
	// Set the status and the running flag. Possible values here are
	// Possible values:
	//   "STATUS_UNKNOWN" - Status of the build is unknown.
	//   "PENDING" - Build has been created and is pending execution and queuing. It has not been queued.
	//   "QUEUED" - Build or step is queued; work has not yet begun.
	//   "WORKING" - Build or step is being executed.
	//   "SUCCESS" - Build or step finished successfully.
	//   "FAILURE" - Build or step failed to complete successfully.
	//   "INTERNAL_ERROR" - Build or step failed due to an internal cause.
	//   "TIMEOUT" - Build or step took longer than was allowed.
	//   "CANCELLED" - Build or step was canceled by a user.
	//   "EXPIRED" - Build was enqueued for longer than the value of
	switch build.Status {
	case "SUCCESS":
		r.IsSuccess = true
		r.IsRunning = false
	case "PENDING", "QUEUED", "WORKING":
		r.IsSuccess = false
		r.IsRunning = true
	case "FAILURE", "INTERNAL_ERROR", "TIMEOUT", "CANCELLED", "EXPIRED":
		r.IsSuccess = false
		r.IsRunning = false
	}

	r.SystemData = build

	return nil
}

// BuildPredicate returns a SLSA predicate populated with the GCB
// run data as recommended by the SLSA 0.2 spec
func (gcb *GCB) BuildPredicate(r *run.Run, draft attestation.Predicate) (predicate attestation.Predicate, err error) {
	type stepData struct {
		Image     string   `json:"image"`
		Arguments []string `json:"arguments"`
	}

	if draft == nil {
		pred := attestation.NewSLSAPredicate()
		predicate = pred
	} else {
		logrus.Debug("Reusing existing slsa predicate")
		predicate = draft
	}
	predicate.SetBuilderType("https://cloudbuild.googleapis.com/CloudBuildYaml@v1")
	buildconfig := map[string]any{}

	steps := []stepData{}

	for _, s := range r.Steps {
		buildconfig["steps"] = append(steps, stepData{
			Image:     s.Image,
			Arguments: s.Params,
		})
	}
	buildconfig["steps"] = steps

	predicate.SetBuildConfig(buildconfig)

	// Get the platform specific data
	build, ok := r.SystemData.(*cloudbuild.Build)
	if ok {
		confsource := &v1.ResourceDescriptor{}
		if build.Substitutions != nil {
			if c, ok := build.Substitutions["COMMIT_SHA"]; ok {
				confsource.Digest["sha1"] = c
				confsource.Digest["gitCommit"] = c
			}
			if t, ok := build.Substitutions["TRIGGER_BUILD_CONFIG_PATH"]; ok {
				predicate.SetEntryPoint(t)
			}
			if _, ok := build.Substitutions["REPO_NAME"]; ok {
				confsource.Uri = fmt.Sprintf(
					"git+https://source.developers.google.com/p/%s/r/%s",
					gcb.ProjectID, build.Substitutions["REPO_NAME"],
				)
			}
		}

		if build.ServiceAccount != "" {
			predicate.SetBuilderID(build.ServiceAccount)
		}

		// Check if we can extract the original repository from the trigger
		if build.BuildTriggerId != "" {
			repo, err := gcb.TriggerDetails(build.BuildTriggerId)
			if err == nil {
				confsource.Uri = repo
			} else {
				logrus.Error(fmt.Errorf("fetching trigger details: %w", err))
			}
		}
		predicate.SetConfigSource(confsource)
	}

	// TODO: review this
	// (*predicate).Invocation.ConfigSource.Digest = build.Substitutions["COMMI"]
	return predicate, nil
}

// TriggerDetails
func (gcb *GCB) TriggerDetails(triggerID string) (repoURL string, err error) {
	cloudbuildService, err := cloudbuild.NewService(context.Background())
	if err != nil {
		return repoURL, fmt.Errorf("creating cloudbuild client: %w", err)
	}
	trigger, err := cloudbuildService.Projects.Triggers.Get(gcb.ProjectID, triggerID).Do()
	if err != nil {
		return repoURL, fmt.Errorf("getting trigger %s from GCB: %w", triggerID, err)
	}

	// If it's a GitHub repository,return the repo URL
	if trigger.Github != nil {
		return fmt.Sprintf("git+https://github.com/%s/%s", trigger.Github.Owner, trigger.Github.Name), nil
	}

	return "", fmt.Errorf("repository not found in trigger")
}

// ArtifactStores returns the native artifact store of cloud build
func (gcb *GCB) ArtifactStores() []store.Store {
	if gcb.ProjectID == "" && gcb.BuildID == "" {
		logrus.Error("incomplete build data to create artifact store")
		return []store.Store{}
	}
	d, err := store.New(fmt.Sprintf("gcb://%s/%s", gcb.ProjectID, gcb.BuildID))
	if err != nil {
		logrus.Error(err)
	}
	return []store.Store{d}
}
