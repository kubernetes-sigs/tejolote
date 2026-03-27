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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	slsav02 "github.com/in-toto/attestation/go/predicates/provenance/v02"
	intoto "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sigs.k8s.io/release-utils/command"
	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/git"
	"sigs.k8s.io/tejolote/pkg/run"
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
func (r *Run) InvocationData() (*slsav02.Invocation, error) {
	// Invocation
	invocation := &slsav02.Invocation{
		ConfigSource: &slsav02.ConfigSource{},
	}

	// Parameters
	params := []string{r.Command}
	params = append(params, r.Params...)
	paramsIface := make([]interface{}, len(params))
	for i, p := range params {
		paramsIface[i] = p
	}
	ps, err := structpb.NewStruct(map[string]interface{}{
		"command": params[0],
		"args":    paramsIface[1:],
	})
	if err != nil {
		return nil, errors.New("unable to form parameters")
	}
	invocation.Parameters = ps

	// Environment
	envIface := map[string]interface{}{}
	for _, e := range os.Environ() {
		varData := strings.SplitN(e, "=", 2)
		if len(varData) == 2 {
			envIface[varData[0]] = varData[1]
		}
	}
	es, err := structpb.NewStruct(envIface)
	if err != nil {
		return nil, fmt.Errorf("forming environment data: %w", err)
	}
	invocation.Environment = es
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
		invocation.ConfigSource.Uri = url + "@" + commit
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

	// Generate the full attestation
	att := attestation.Attestation{
		Statement: intoto.Statement{
			Type:          intoto.StatementTypeUri,
			Subject:       []*intoto.ResourceDescriptor{},
			PredicateType: predicate.Type(),
		},
		Predicate: predicate,
	}

	// Add the artifacts to the attestation
	for _, m := range r.Artifacts {
		att.Subject = append(att.Subject, &intoto.ResourceDescriptor{
			Name:   m.Path,
			Digest: m.Checksum,
		})
	}

	data, err := att.ToJSON()
	if err != nil {
		return err
	}

	// Create the file
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("opening attestation path %s for writing: %w", path, err)
	}
	defer out.Close()

	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("writing data to file: %w", err)
	}
	return nil
}

func (r *Run) Predicate() (attestation.Predicate, error) {
	invocation, err := r.InvocationData()
	if err != nil {
		return nil, fmt.Errorf("reading invocation data: %w", err)
	}

	predicate := attestation.SLSAPredicate{
		Builder: &slsav02.Builder{
			Id: "", // TODO: Read builder from trsuted environment
		},
		BuildType:   TejoloteURI,
		Invocation:  invocation,
		BuildConfig: nil,
		Metadata: &slsav02.Metadata{
			BuildInvocationId: "",
			BuildStartedOn:    timestamppb.New(r.StartTime),
			BuildFinishedOn:   timestamppb.New(r.EndTime),
			Completeness: &slsav02.Completeness{
				Parameters:  true,
				Environment: false,
				Materials:   false,
			},
			Reproducible: false,
		},
		Materials: []*slsav02.Material{},
	}

	return &predicate, nil
}
