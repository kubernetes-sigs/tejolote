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
	"strings"
	"time"

	intoto "github.com/in-toto/in-toto-golang/in_toto"
	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"
	"github.com/puerco/tejolote/pkg/git"
	"github.com/puerco/tejolote/pkg/run"
	"sigs.k8s.io/release-utils/command"
)

type Run struct {
	Executable  *command.Command
	ExitCode    int
	Artifacts   []run.Artifact
	Output      *command.Stream
	Status      command.Status
	Command     string
	Params      []string
	StartTime   time.Time
	EndTime     time.Time
	Environment RunEnvironment
}

const TejoloteURI = "http://github.com/kubernetes-sigs/tejolote"

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
	invocation.Parameters = []string{r.Command}
	invocation.Parameters = append(invocation.Parameters.([]string), r.Params...)
	invocation.Environment = map[string]string{}

	for _, e := range os.Environ() {
		varData := strings.SplitN(e, "=", 2)
		if len(varData) == 2 {
			invocation.Environment.(map[string]string)[varData[0]] = varData[1]
		}
	}

	// Read the git repo data
	if git.IsRepo(r.Environment.Directory) {
		repo, err := git.NewRepository(r.Environment.Directory)
		if err != nil {
			return invocation, fmt.Errorf("opening build repo: %w", err)
		}
		url, err := repo.SourceURL()
		if err != nil {
			return invocation, fmt.Errorf("opening project repository: %w", err)
		}

		commit, err := repo.HeadCommitSHA()
		if err != nil {
			return invocation, fmt.Errorf("fetching build point commit")
		}
		invocation.ConfigSource.URI = url + "@" + commit
	}

	return invocation, nil
}

// WriteAttestation writes the provenance attestation describing the build
func (r *Run) WriteAttestation(path string) error {
	// Get the predicate
	predicate, err := r.Predicate()
	if err != nil {
		return fmt.Errorf("generating attestation: %w", err)
	}

	attestation := intoto.Statement{
		StatementHeader: intoto.StatementHeader{
			Type:          intoto.StatementInTotoV01,
			PredicateType: slsa.PredicateSLSAProvenance,
			Subject:       []intoto.Subject{},
		},
		Predicate: predicate,
	}

	// Add the artifacts to the attestation
	for _, m := range r.Artifacts {
		attestation.StatementHeader.Subject = append(attestation.StatementHeader.Subject, intoto.Subject{
			Name:   m.Path,
			Digest: m.Checksum,
		})
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

func (r *Run) Predicate() (*slsa.ProvenancePredicate, error) {
	invocation, err := r.InvocationData()
	if err != nil {
		return nil, fmt.Errorf("reading invocation data: %w", err)
	}
	predicate := slsa.ProvenancePredicate{
		Builder: slsa.ProvenanceBuilder{
			ID: "", // TODO: Read builder from trsuted environment
		},
		BuildType:   TejoloteURI,
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

	return &predicate, nil
}
