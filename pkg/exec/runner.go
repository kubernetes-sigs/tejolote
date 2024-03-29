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

package exec

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/watcher"
)

func NewRunner() *Runner {
	return &Runner{
		Options: Options{
			Logger: logrus.New(),
		},
		implementation: &defaultRunnerImplementation{},
		Watchers:       []watcher.Watcher{},
	}
}

type Runner struct {
	Options        Options
	implementation RunnerImplementation
	Watchers       []watcher.Watcher
}

type Options struct {
	Verbose         bool
	CWD             string
	AttestationPath string
	Logger          *logrus.Logger
}

// RunStep executes a step
func (r *Runner) RunStep(step *run.Step) (runner *Run, err error) {
	// Create the command
	runner, err = r.implementation.CreateRun(&r.Options, step)
	if err != nil {
		return nil, fmt.Errorf("getting step command and arguments: %w", err)
	}

	// Call the watcher to snapshot everything
	if err := r.implementation.Snapshot(&r.Options, &r.Watchers); err != nil {
		return runner, fmt.Errorf("running initial snapshots: %w", err)
	}

	if err := r.implementation.Execute(&r.Options, runner); err != nil {
		return nil, fmt.Errorf("executing run: %w", err)
	}

	// Call the watcher to snapshot the results
	if err := r.implementation.Snapshot(&r.Options, &r.Watchers); err != nil {
		return runner, fmt.Errorf("running final snapshots: %w", err)
	}

	// TODO: review this
	//nolint: gocritic
	/*
		for _, w := range r.Watchers {
			run.Artifacts = append(run.Artifacts, w.(*watcher.Directory).Snapshots[0].Delta(&w.(*watcher.Directory).Snapshots[1])...)
		}
	*/
	if err := r.implementation.WriteAttestation(&r.Options, runner); err != nil {
		return runner, fmt.Errorf("writing provenance attestation: %w", err)
	}

	return runner, err
}
