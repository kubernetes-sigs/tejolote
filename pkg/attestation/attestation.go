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

	intoto "github.com/in-toto/in-toto-golang/in_toto"
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
