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
	"fmt"
	"slices"

	"github.com/carabiner-dev/signer"
)

var EnvelopeFormats = []string{"bundle", "dsse"}

// Sign signs the attestation with sigstore and returns a serialized Sigstore
// bundle: the in-toto statement wrapped in a DSSE envelope together with the
// Fulcio signing certificate and the Rekor transparency-log inclusion proof,
// all in a single self-contained, verifiable file.
//
// Signing uses the keyless sigstore flow with ambient credentials (e.g. the
// GitHub Actions or SPIFFE workload identity) when available. The signer library
// handles requesting the Fulcio certificate and registering the entry in the Rekor
// transparency log unlike the previous detached-DSSE output, which embedded neither
// the certificate nor a transparency-log proof and was therefore not independently
// verifiable.
func (att *Attestation) Sign(envelopeFormat string) ([]byte, error) {
	if !slices.Contains(EnvelopeFormats, envelopeFormat) {
		return nil, fmt.Errorf("invalid envelope format, tejolote supports: %v", EnvelopeFormats)
	}

	statement, err := att.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("serializing attestation to json: %w", err)
	}

	s := signer.NewSigner()

	bndl, err := s.SignStatementBundle(statement)
	if err != nil {
		return nil, fmt.Errorf("signing attestation as sigstore bundle: %w", err)
	}

	var buf bytes.Buffer
	switch envelopeFormat {
	case "", "bundle":
		if err := s.WriteBundle(bndl, &buf); err != nil {
			return nil, fmt.Errorf("marshaling sigstore bundle: %w", err)
		}
	case "dsse":
		// Not recommended: strip the signed material down to the bare DSSE
		// envelope carried inside the bundle, dropping the Fulcio certificate
		// and the Rekor inclusion proof. Only for consumers that cannot read
		// sigstore bundles yet.
		env := bndl.GetDsseEnvelope()
		if env == nil {
			return nil, fmt.Errorf("signed bundle contains no DSSE envelope")
		}
		if err := s.WriteDSSEEnvelope(env, &buf); err != nil {
			return nil, fmt.Errorf("marshaling dsse envelope: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown envelope format %q (want \"bundle\" or \"dsse\")", envelopeFormat)
	}
	return buf.Bytes(), nil
}
