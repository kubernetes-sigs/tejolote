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

package driver

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/protobom/protobom/pkg/reader"
	"github.com/protobom/protobom/pkg/sbom"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type SPDX struct {
	URL string
}

// NewSPDX creates a new SPDX storage
func NewSPDX(specURL string) (*SPDX, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing attestation spec url: %w", err)
	}
	if !strings.HasPrefix(u.Scheme, "spdx+") {
		return nil, fmt.Errorf("spec URL %s is not an attestation url", u.Scheme)
	}

	logrus.Infof(
		"Initialized new SPDX SBOM storage backend (%s)", specURL,
	)

	// TODO(puerco): Check scheme to make sure it is valid
	return &SPDX{
		URL: strings.TrimPrefix(specURL, "spdx+"),
	}, nil
}

func (s *SPDX) Snap() (*snapshot.Snapshot, error) {
	f, err := os.CreateTemp("", "temp-sbom-")
	if err != nil {
		return nil, fmt.Errorf("creating temporary sbom file: %w", err)
	}
	defer os.Remove(f.Name())

	if err := downloadURL(s.URL, f); err != nil {
		return nil, fmt.Errorf("downloading sbom to temp file: %w", err)
	}

	doc, err := reader.New().ParseFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("parsing SBOM: %w", err)
	}

	snap := snapshot.Snapshot{}

	// Add the spdx packages
	for _, node := range doc.GetRootNodes() {
		// First, check to see if the package has a purl
		identifier := string(node.Purl())

		// If not, try download location
		if identifier == "" && node.GetUrlDownload() != "" {
			identifier = node.GetUrlDownload()
		}

		// If else fails, use the package name
		// TODO(puerco): We should rather expand artifact to be a full resource
		// descriptor and put the name there. Oh well.
		if identifier == "" {
			identifier = node.GetName()
		}

		// Should we list packages without checksums?
		// Leaving this commented because it breaks with the kubernetes sboms
		// but perhaps we should be stricter here
		if len(node.GetHashes()) == 0 {
			logrus.Warnf("SPDX package %s has no checksum", identifier)
			continue
		}

		artifact := run.Artifact{
			Path:     identifier,
			Checksum: map[string]string{},
		}
		for algoID, value := range node.GetHashes() {
			artifact.Checksum[sbom.HashAlgorithm(algoID).String()] = value
		}

		snap[identifier] = artifact
	}
	return &snap, nil
}
