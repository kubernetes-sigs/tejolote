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

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"
	"github.com/puerco/tejolote/pkg/git"
	"github.com/puerco/tejolote/pkg/watcher"
	"sigs.k8s.io/release-utils/command"
)

type Run struct {
	Executable  *command.Command
	ExitCode    int
	Artifacts   []watcher.Artifact
	Output      *command.Stream
	Status      command.Status
	Command     string
	Params      []string
	StartTime   time.Time
	EndTime     time.Time
	Environment RunEnvironment
}

type RunEnvironment struct {
	Variables map[string]string
	Directory string
}

// InvocationData return the invocation of the command in SLSA strcut
func (r *Run) InvocationData() (slsa.ProvenanceInvocation, error) {
	// Get the git drector
	invocation := slsa.ProvenanceInvocation{
		ConfigSource: slsa.ConfigSource{},
	}
	invocation.Parameters = r.Params
	invocation.Environment = r.Environment.Variables

	// Read the git repo data
	repo := git.NewRepository(r.Environment.Directory)
	url, err := repo.SourceURL()
	if err != nil {
		return invocation, fmt.Errorf("opening project repository: %w", err)
	}
	invocation.ConfigSource.URI = url

	return invocation, nil
}

func (r *Run) WriteAttestation(path string) error {
	attestation, err := r.Attest()
	if err != nil {
		return fmt.Errorf("generating attestation: %w", err)
	}

	// Create the file
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("opening attestation path %s for writing: %w", path, err)
	}
	defer out.Close()

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(attestation); err != nil {
		return fmt.Errorf("encoding spdx sbom: %w", err)
	}
	return nil
}

func (r *Run) Attest() (*slsa.ProvenancePredicate, error) {
	invocation, err := r.InvocationData()
	if err != nil {
		return nil, fmt.Errorf("reading invocation data: %w", err)
	}
	predicate := slsa.ProvenancePredicate{
		Builder: slsa.ProvenanceBuilder{
			ID: "", // TODO: Read builder from trsuted environment
		},
		BuildType:   "",
		Invocation:  invocation,
		BuildConfig: nil,
		Metadata: &slsa.ProvenanceMetadata{
			BuildInvocationID: "",
			BuildStartedOn:    &r.StartTime,
			BuildFinishedOn:   &r.EndTime,
			Completeness: slsa.ProvenanceComplete{
				Parameters:  true,
				Environment: false,
				Materials:   false,
			},
			Reproducible: false,
		},
		Materials: []slsa.ProvenanceMaterial{},
	}
	for _, m := range r.Artifacts {
		predicate.Materials = append(predicate.Materials, slsa.ProvenanceMaterial{
			URI: m.Path,
			Digest: map[string]string{
				"sha256": m.Hash,
			},
		})
	}
	return &predicate, nil
}
