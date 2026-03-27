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
	"encoding/json"
	"fmt"

	intoto "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

type (
	Attestation struct {
		intoto.Statement
		Predicate Predicate `json:"predicate"`
	}
)

func New() *Attestation {
	attestation := &Attestation{
		Statement: intoto.Statement{
			Type:    intoto.StatementTypeUri,
			Subject: []*intoto.ResourceDescriptor{},
		},
		Predicate: nil,
	}
	return attestation
}

func (att *Attestation) SLSA() *Attestation {
	att.Predicate = NewSLSAPredicate()
	att.PredicateType = att.Predicate.Type()
	return att
}

func (att *Attestation) SLSAv1() *Attestation {
	att.Predicate = NewSLSAV1Predicate()
	att.PredicateType = att.Predicate.Type()
	return att
}

func (att *Attestation) ToJSON() ([]byte, error) {
	// Convert the typed Predicate into a *structpb.Struct so it is
	// included in the protobuf Statement serialization.
	if att.Predicate != nil {
		predJSON, err := json.Marshal(att.Predicate)
		if err != nil {
			return nil, fmt.Errorf("marshaling predicate to JSON: %w", err)
		}
		var predMap map[string]interface{}
		if err := json.Unmarshal(predJSON, &predMap); err != nil {
			return nil, fmt.Errorf("unmarshaling predicate JSON: %w", err)
		}
		s, err := structpb.NewStruct(predMap)
		if err != nil {
			return nil, fmt.Errorf("converting predicate to structpb: %w", err)
		}
		att.Statement.Predicate = s
	}

	m := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}

	jsonData, err := m.Marshal(&att.Statement)
	if err != nil {
		return nil, fmt.Errorf("marshaling attestation: %w", err)
	}

	return jsonData, nil
}
