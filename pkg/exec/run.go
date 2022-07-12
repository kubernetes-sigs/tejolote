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
	"github.com/puerco/tejolote/pkg/watcher"
	"sigs.k8s.io/release-utils/command"
)

type Run struct {
	Executable *command.Command
	ExitCode   int
	Artifacts  []watcher.Artifact
	Output     *command.Stream
	Status     command.Status
	Command    string
	Params     []string
	StartTime  time.Time
	EndTime    time.Time
}

func (r *Run) WriteAttestation(path string) error {
	attestation := r.Attest()

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

func (r *Run) Attest() slsa.ProvenancePredicate {
	predicate := slsa.ProvenancePredicate{
		Builder: slsa.ProvenanceBuilder{
			ID: "",
		},
		BuildType: "",
		Invocation: slsa.ProvenanceInvocation{
			ConfigSource: slsa.ConfigSource{
				URI:        "",
				Digest:     map[string]string{},
				EntryPoint: "",
			},
			Parameters:  nil,
			Environment: nil,
		},
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
	return predicate
}
