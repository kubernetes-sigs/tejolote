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

package attestation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/in-toto/attestation/go/v1"
	intoto "github.com/in-toto/in-toto-golang/in_toto"
	"github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/common"
	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"
)

type (
	Attestation struct {
		intoto.StatementHeader
		Predicate Predicate `json:"predicate"`
	}
	SLSAPredicate slsa.ProvenancePredicate
)

func New() *Attestation {
	attestation := &Attestation{
		StatementHeader: intoto.StatementHeader{
			Type:          intoto.StatementInTotoV01,
			PredicateType: slsa.PredicateSLSAProvenance,
			Subject:       []intoto.Subject{},
		},
	}
	return attestation
}

func (att *Attestation) SLSA() *Attestation {
	att.Predicate = NewSLSAPredicate()
	return att
}

func (att *Attestation) SLSAv1() *Attestation {
	att.Predicate = NewSLSAV1Predicate()
	return att
}

// NewSLSAPredicate returns a new SLSA predicate fully initialized
func NewSLSAPredicate() *SLSAPredicate {
	predicate := &SLSAPredicate{
		Builder: common.ProvenanceBuilder{
			ID: "", // TODO: Read builder from trusted environment
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
			BuildStartedOn:    nil,
			BuildFinishedOn:   nil,
			Completeness: slsa.ProvenanceComplete{
				Parameters:  true,
				Environment: false,
				Materials:   false,
			},
			Reproducible: false,
		},
		Materials: []common.ProvenanceMaterial{},
	}

	return predicate
}

func (pred *SLSAPredicate) SetBuilderID(id string) {
	pred.Builder.ID = id
}

func (pred *SLSAPredicate) SetBuilderType(id string) {
	pred.BuildType = id
}

func (pred *SLSAPredicate) SetInvocationID(id string) {
	pred.Metadata.BuildInvocationID = id
}

func (pred *SLSAPredicate) SetConfigSource(src *v1.ResourceDescriptor) {
	for algo, val := range src.GetDigest() {
		pred.Invocation.ConfigSource.Digest[algo] = val
	}
	pred.Invocation.ConfigSource.URI = src.GetUri()
}

func (pred *SLSAPredicate) SetEntryPoint(ep string) {
	pred.Invocation.ConfigSource.EntryPoint = ep
}

func (pred *SLSAPredicate) SetResolvedDependencies(deps []*v1.ResourceDescriptor) {
	pred.Materials = []common.ProvenanceMaterial{}
	for _, dep := range deps {
		pred.Materials = append(pred.Materials, common.ProvenanceMaterial{
			URI:    dep.GetUri(),
			Digest: dep.GetDigest(),
		})
	}
}

func (att *Attestation) ToJSON() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(att); err != nil {
		return nil, fmt.Errorf("encoding spdx sbom: %w", err)
	}
	return b.Bytes(), nil
}

// AddMaterial add an entry to the materials
func (pred *SLSAPredicate) AddDependency(dep *v1.ResourceDescriptor) {
	if pred.Materials == nil {
		pred.Materials = []common.ProvenanceMaterial{}
	}
	mat := common.ProvenanceMaterial{
		URI:    dep.GetUri(),
		Digest: dep.GetDigest(),
	}
	for i, m := range pred.Materials {
		if m.URI == dep.GetUri() {
			pred.Materials[i] = mat
			return
		}
	}
	pred.Materials = append(pred.Materials, mat)
}

func (pred *SLSAPredicate) SetBuildConfig(conf map[string]any) {
	pred.BuildConfig = conf
}

func (pred *SLSAPredicate) SetInternalParameters(params map[string]any) {
	pred.Invocation.Environment = params
}

func (pred *SLSAPredicate) SetStartedOn(d *time.Time) {
	pred.Metadata.BuildStartedOn = d
}

func (pred *SLSAPredicate) SetFinishedOn(d *time.Time) {
	pred.Metadata.BuildFinishedOn = d
}
