/*
Copyright 2026 The Kubernetes Authors.

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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/carabiner-dev/signer"
	"github.com/carabiner-dev/signer/key"
	intoto "github.com/in-toto/attestation/go/v1"
	sdsse "github.com/sigstore/protobuf-specs/gen/pb-go/dsse"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// testKeyPair generates an ECDSA P-256 key pair, the kind a CI job would
// create with `openssl genpkey`, and returns the private key as a signing
// provider and the public key for verification.
func testKeyPair(t *testing.T) (key.PrivateKeyProvider, key.PublicKeyProvider) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)

	parser := key.NewParser()
	signingKey, err := parser.ParsePrivateKeyProvider(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}))
	require.NoError(t, err)
	verifyKey, err := parser.ParsePublicKeyProvider(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	require.NoError(t, err)

	return signingKey, verifyKey
}

func TestSignWithKeys(t *testing.T) {
	att := New().SLSAv1()
	att.Subject = append(att.Subject, &intoto.ResourceDescriptor{
		Name:   "artifact.txt",
		Digest: map[string]string{"sha256": "0000000000000000000000000000000000000000000000000000000000000000"},
	})

	signingKey, verifyKey := testKeyPair(t)

	data, err := att.SignWithKeys(signingKey)
	require.NoError(t, err)

	env := &sdsse.Envelope{}
	require.NoError(t, protojson.Unmarshal(data, env))
	require.Equal(t, "https://in-toto.io/Statement/v1", env.GetPayloadType())
	require.Len(t, env.GetSignatures(), 1)

	result, err := signer.NewVerifier().VerifyParsedDSSE(env, []key.PublicKeyProvider{verifyKey})
	require.NoError(t, err)
	require.True(t, result.Verified, "envelope must verify against the signing key's public key")

	_, otherKey := testKeyPair(t)
	result, err = signer.NewVerifier().VerifyParsedDSSE(env, []key.PublicKeyProvider{otherKey})
	if err == nil {
		require.False(t, result.Verified, "envelope must not verify against an unrelated key")
	}
}

func TestSignWithKeysRequiresKey(t *testing.T) {
	_, err := New().SLSAv1().SignWithKeys()
	require.Error(t, err)
}
