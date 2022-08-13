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

	"github.com/puerco/tejolote/pkg/run"

	"github.com/sirupsen/logrus"
	"google.golang.org/api/cloudbuild/v1"
)

type GCB struct {
	ProjectID string
	BuildID   string
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

	/*
		req, err := cloudbuildService.Projects.Builds.List(project).Filter(tagsFilter).PageSize(50).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to listing the builds: %w", err)
		}
	*/
}

func (gcb *GCB) RefreshRun(r *run.Run) error {
	// Fetch the required data to get the build from the URL
	u, err := url.Parse(r.SpecURL)
	if err != nil {
		return fmt.Errorf("parsing GCB spec URL: %w", err)
	}

	project := u.Hostname()
	buildID := strings.TrimPrefix(u.Path, "/")

	ctx := context.Background()
	cloudbuildService, err := cloudbuild.NewService(ctx)
	if err != nil {
		return fmt.Errorf("creating cloudbuild client: %w", err)
	}
	build, err := cloudbuildService.Projects.Builds.Get(project, buildID).Do()
	if err != nil {
		return fmt.Errorf("getting build %s from GCB: %w", buildID, err)
	}
	logrus.Infof("%+v", build)

	for i, s := range build.Steps {
		logrus.Infof("Step #%d %+v", i, s)
	}
	// Set the status and the running flag
	if build.Status == "SUCCESS" {
		r.IsSuccess = true
		r.IsRunning = false
	}

	return nil
}

/*
type Options struct {
	ConfigFile string
	Step       int
}

func NewFromConfig()

type GCB struct {
}

type Config struct {
	Steps []Step `json:"steps"`
	Tags []string `json:"tags"`
}

type Step struct {
	Name string `json:"name"`
	Dir string `json:"name"`
	Args []string `json:"args"`
	SecretEnv []string `json:"secretEnv"`
	Environment []string `json:"env"` // these are  LABEL=value strings
}




secrets:
- kmsKeyName: projects/k8s-releng-prod/locations/global/keyRings/release/cryptoKeys/encrypt-0
  secretEnv:
    GITHUB_TOKEN: CiQAIkW
    DOCKERHUB_TOKEN: CiQA

steps:
- name: gcr.io/cloud-builders/git
  dir: "go/src/k8s.io"
  args:
  - "clone"
  - "https://github.com/${_TOOL_ORG}/${_TOOL_REPO}"

- name: gcr.io/cloud-builders/git
  entrypoint: "bash"
  dir: "go/src/k8s.io/release"
  args:
  - '-c'
  - |
    git fetch
    echo "Checking out ${_TOOL_REF}"
    git checkout ${_TOOL_REF}
- name: gcr.io/k8s-staging-releng/k8s-cloud-builder:${_KUBE_CROSS_VERSION_LATEST}
  dir: "go/src/k8s.io/release"
  env:
  - "GOPATH=/workspace/go"
  - "GOBIN=/workspace/bin"
  args:
  - "./compile-release-tools"
  - "krel"

- name: gcr.io/k8s-staging-releng/k8s-cloud-builder:${_KUBE_CROSS_VERSION}
  dir: "/workspace"
  env:
  - "TOOL_ORG=${_TOOL_ORG}"
  - "TOOL_REPO=${_TOOL_REPO}"
  - "TOOL_REF=${_TOOL_REF}"
  - "BUILD_ID=${BUILD_ID}"
  - "K8S_ORG=${_K8S_ORG}"
  - "K8S_REPO=${_K8S_REPO}"
  - "K8S_REF=${_K8S_REF}"
  - GOOGLE_SERVICE_ACCOUNT_NAME=krel-staging@k8s-releng-prod.iam.gserviceaccount.com
  secretEnv:
  - GITHUB_TOKEN
  - DOCKERHUB_TOKEN
  args:
  - "bin/krel"
  - "stage"
  - "--submit=false"
  - "${_NOMOCK}"
  - "--log-level=${_LOG_LEVEL}"
  - "--type=${_TYPE}"
  - "--branch=${_RELEASE_BRANCH}"
  - "--build-version=${_BUILDVERSION}"

tags:
- ${_GCP_USER_TAG}
- ${_RELEASE_BRANCH}
- ${_NOMOCK_TAG}
- STAGE
- ${_GIT_TAG}
- ${_TYPE_TAG}
- ${_MAJOR_VERSION_TAG}
- ${_MINOR_VERSION_TAG}
- ${_PATCH_VERSION_TAG}
- ${_KUBERNETES_VERSION_TAG}

options:
  machineType: N1_HIGHCPU_32

substitutions:
  # _GIT_TAG will be filled with a git-based tag of the form vYYYYMMDD-hash, and
  # can be used as a substitution
  _GIT_TAG: '12345'


*/
