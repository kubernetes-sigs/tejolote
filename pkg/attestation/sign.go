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
	"context"
	"fmt"
	"time"

	"github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/cmd/cosign/cli/sign"
	"github.com/sigstore/sigstore/pkg/signature/dsse"
	signatureoptions "github.com/sigstore/sigstore/pkg/signature/options"
)

func (att *Attestation) Sign() ([]byte, error) {
	ctx := context.Background()
	var timeout time.Duration /// TODO move to options
	var certPath, certChainPath string
	ko := options.KeyOpts{
		// KeyRef:     s.options.PrivateKeyPath,
		// IDToken:    identityToken,
		FulcioURL:    options.DefaultFulcioURL,
		RekorURL:     options.DefaultRekorURL,
		OIDCIssuer:   options.DefaultOIDCIssuerURL,
		OIDCClientID: "sigstore",

		InsecureSkipFulcioVerify: false,
		SkipConfirmation:         true,
		// FulcioAuthFlow:           "",
	}
	/*
		if options.EnableExperimental() {
			if options.NOf(ko.KeyRef, ko.Sk) > 1 {
				return &options.KeyParseError{}
			}
		} else {
			if !options.OneOf(ko.KeyRef, ko.Sk) {
				return &options.KeyParseError{}
			}
		}
	*/
	if timeout != 0 {
		var cancelFn context.CancelFunc
		ctx, cancelFn = context.WithTimeout(ctx, timeout)
		defer cancelFn()
	}

	sv, err := sign.SignerFromKeyOpts(ctx, certPath, certChainPath, ko)
	if err != nil {
		return nil, fmt.Errorf("getting signer: %w", err)
	}
	defer sv.Close()

	// Wrap the attestation in the DSSE envelope
	wrapped := dsse.WrapSigner(sv, "application/vnd.in-toto+json")

	json, err := att.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("serializing attestation to json: %w", err)
	}

	signedPayload, err := wrapped.SignMessage(
		bytes.NewReader(json), signatureoptions.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("signing attestation: %w", err)
	}

	fmt.Println(string(signedPayload))
	return signedPayload, nil

	// ???
	/*
		opts := []static.Option{static.WithLayerMediaType(types.DssePayloadType)}
		if sv.Cert != nil {
			opts = append(opts, static.WithCertChain(sv.Cert, sv.Chain))
		}
	*/
	// Should we upload?
	/*
		// Check whether we should be uploading to the transparency log
		if sign.ShouldUploadToTlog(ctx, digest, force, noTlogUpload, ko.RekorURL) {
			bundle, err := uploadToTlog(ctx, sv, ko.RekorURL, func(r *client.Rekor, b []byte) (*models.LogEntryAnon, error) {
				return cosign.TLogUploadInTotoAttestation(ctx, r, signedPayload, b)
			})
			if err != nil {
				return err
			}
			opts = append(opts, static.WithBundle(bundle))
		}
	*/
}
