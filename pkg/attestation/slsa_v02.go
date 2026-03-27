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
	"time"

	slsav02 "github.com/in-toto/attestation/go/predicates/provenance/v02"
	v1 "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SLSAPredicate slsav02.Provenance

// NewSLSAPredicate returns a new SLSA predicate fully initialized
func NewSLSAPredicate() *SLSAPredicate {
	predicate := &SLSAPredicate{
		Builder: &slsav02.Builder{
			Id: "",
		},
		BuildType: "",
		Invocation: &slsav02.Invocation{
			ConfigSource: &slsav02.ConfigSource{
				Uri:        "",
				Digest:     map[string]string{},
				EntryPoint: "",
			},
		},
		BuildConfig: nil,
		Metadata: &slsav02.Metadata{
			BuildInvocationId: "",
			Completeness: &slsav02.Completeness{
				Parameters:  true,
				Environment: false,
				Materials:   false,
			},
			Reproducible: false,
		},
		Materials: []*slsav02.Material{},
	}

	return predicate
}

func (pred *SLSAPredicate) SetBuilderID(id string) {
	pred.Builder.Id = id
}

func (pred *SLSAPredicate) SetBuilderType(id string) {
	pred.BuildType = id
}

func (pred *SLSAPredicate) SetInvocationID(id string) {
	pred.Metadata.BuildInvocationId = id
}

func (pred *SLSAPredicate) SetConfigSource(src *v1.ResourceDescriptor) {
	for algo, val := range src.GetDigest() {
		pred.Invocation.ConfigSource.Digest[algo] = val
	}
	pred.Invocation.ConfigSource.Uri = src.GetUri()
}

func (pred *SLSAPredicate) SetEntryPoint(ep string) {
	pred.Invocation.ConfigSource.EntryPoint = ep
}

func (pred *SLSAPredicate) SetResolvedDependencies(deps []*v1.ResourceDescriptor) {
	pred.Materials = []*slsav02.Material{}
	for _, dep := range deps {
		pred.Materials = append(pred.Materials, &slsav02.Material{
			Uri:    dep.GetUri(),
			Digest: dep.GetDigest(),
		})
	}
}

// AddMaterial add an entry to the materials
func (pred *SLSAPredicate) AddDependency(dep *v1.ResourceDescriptor) {
	if pred.Materials == nil {
		pred.Materials = []*slsav02.Material{}
	}
	mat := &slsav02.Material{
		Uri:    dep.GetUri(),
		Digest: dep.GetDigest(),
	}
	for i, m := range pred.Materials {
		if m.GetUri() == dep.GetUri() {
			pred.Materials[i] = mat
			return
		}
	}
	pred.Materials = append(pred.Materials, mat)
}

func (pred *SLSAPredicate) SetBuildConfig(conf map[string]any) {
	s, err := structpb.NewStruct(conf)
	if err != nil {
		return
	}
	pred.BuildConfig = s
}

func (pred *SLSAPredicate) SetInternalParameters(params map[string]any) {
	s, err := structpb.NewStruct(params)
	if err != nil {
		return
	}
	pred.Invocation.Environment = s
}

func (pred *SLSAPredicate) SetStartedOn(d *time.Time) {
	if d == nil {
		return
	}
	pred.Metadata.BuildStartedOn = timestamppb.New(*d)
}

func (pred *SLSAPredicate) SetFinishedOn(d *time.Time) {
	if d == nil {
		return
	}
	pred.Metadata.BuildFinishedOn = timestamppb.New(*d)
}

func (pred *SLSAPredicate) Type() string {
	return "https://slsa.dev/provenance/v0.2"
}

func (pred *SLSAPredicate) MarshalJSON() ([]byte, error) {
	p := (*slsav02.Provenance)(pred)
	return protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(p)
}

// This predicate type does not support external params
func (pred *SLSAPredicate) AddExternalParameter(string, any) {
}
