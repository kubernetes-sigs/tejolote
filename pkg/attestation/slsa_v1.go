/*
Copyright 2024 The Kubernetes Authors.

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

	slsa1 "github.com/in-toto/attestation/go/predicates/provenance/v1"
	v1 "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SLSAPredicateV1 struct {
	slsa1.Provenance
}

func NewSLSAV1Predicate() *SLSAPredicateV1 {
	return &SLSAPredicateV1{
		Provenance: slsa1.Provenance{
			BuildDefinition: &slsa1.BuildDefinition{
				BuildType: "",
				ExternalParameters: &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				},
				InternalParameters: &structpb.Struct{
					Fields: map[string]*structpb.Value{},
				},
				ResolvedDependencies: []*v1.ResourceDescriptor{},
			},
			RunDetails: &slsa1.RunDetails{
				Builder: &slsa1.Builder{
					Id:      "",
					Version: map[string]string{},
					// BuilderDependencies: []*v1.ResourceDescriptor{},
				},
				Metadata: &slsa1.BuildMetadata{
					InvocationId: "",
					StartedOn:    &timestamppb.Timestamp{},
					FinishedOn:   &timestamppb.Timestamp{},
				},
				Byproducts: []*v1.ResourceDescriptor{},
			},
		},
	}
}

func (pred *SLSAPredicateV1) SetBuilderID(id string) {
	pred.RunDetails.Builder.Id = id
}

func (pred *SLSAPredicateV1) SetBuilderType(id string) {
	pred.BuildDefinition.BuildType = id
}

func (pred *SLSAPredicateV1) SetInvocationID(id string) {
	pred.RunDetails.Metadata.InvocationId = id
}

func (pred *SLSAPredicateV1) SetConfigSource(src *v1.ResourceDescriptor) {
	lc8r := src.GetUri()
	if h, ok := src.GetDigest()["sha1"]; ok && h != "" {
		lc8r += "@" + h
	}
	pred.BuildDefinition.ExternalParameters.Fields["source"] = structpb.NewStringValue(lc8r)
}

func (pred *SLSAPredicateV1) SetEntryPoint(ep string) {
	pred.BuildDefinition.ExternalParameters.Fields["entryPoint"] = structpb.NewStringValue(ep)
}

func (pred *SLSAPredicateV1) SetResolvedDependencies(deps []*v1.ResourceDescriptor) {
	// Todo, here we need to add:
	// {
	//     "uri": old.invocation.configSource.uri,
	//     "digest": old.invocation.configSource.digest,
	// }
	// Which now lives in external parameters plus other stuff used
	// (see above)
	pred.BuildDefinition.ResolvedDependencies = deps
}

// SetBuildConfig is deprecated in v1:
//
//	buildConfig: No longer inlined into the provenance. Instead, either:
//	If the configuration is a top-level input, record its digest in
//	externalParameters["config"].
//
//	Else if there is a known use case for knowing the exact resolved build
//	configuration, record its digest in byproducts. An example use case
//	might be someone who wishes to parse the configuration to look for bad
//	patterns, such as curl | bash.
//
//	Else omit it.
func (pred *SLSAPredicateV1) SetBuildConfig(conf map[string]any) {}

func (pred *SLSAPredicateV1) SetInternalParameters(params map[string]any) {
	s, err := structpb.NewStruct(params)
	if err != nil {
		return
	}
	pred.BuildDefinition.InternalParameters = s
}

func (pred *SLSAPredicateV1) AddDependency(dep *v1.ResourceDescriptor) {
	pred.BuildDefinition.ResolvedDependencies = append(pred.BuildDefinition.ResolvedDependencies, dep)
}

func (pred *SLSAPredicateV1) MarshalJSON() ([]byte, error) {
	return protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(pred)
}

func (pred *SLSAPredicateV1) SetStartedOn(d *time.Time) {
	if d == nil {
		pred.RunDetails.Metadata.StartedOn = nil
		return
	}
	pred.RunDetails.Metadata.StartedOn = timestamppb.New(*d)
}

func (pred *SLSAPredicateV1) SetFinishedOn(d *time.Time) {
	if d == nil {
		pred.RunDetails.Metadata.FinishedOn = nil
		return
	}
	pred.RunDetails.Metadata.FinishedOn = timestamppb.New(*d)
}

func (pred *SLSAPredicateV1) Type() string {
	return "https://slsa.dev/provenance/v1"
}
