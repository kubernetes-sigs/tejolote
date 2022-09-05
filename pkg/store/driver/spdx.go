/*
Copyright 2022 Adolfo García Veytia

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

	"github.com/puerco/tejolote/pkg/run"
	"github.com/puerco/tejolote/pkg/store/snapshot"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/bom/pkg/spdx"
)

type SPDX struct {
	URL string
}

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

	// TODO: Check scheme to make sure it is valid
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

	doc, err := spdx.OpenDoc(f.Name())
	if err != nil {
		return nil, fmt.Errorf("parsing spdx sbom: %w", err)
	}

	snap := snapshot.Snapshot{}

	// Add the spdx packages
	for _, p := range doc.Packages {
		// First, check to see if the SBOM has a purl
		identifier := ""
		for _, ref := range p.ExternalRefs {
			if ref.Type == "purl" {
				identifier = ref.Locator
				break
			}
		}

		// If not, try download location
		if identifier == "" && p.DownloadLocation != "" {
			identifier = p.DownloadLocation
		}

		// If else fails, use the package name
		// TODO: Think if this works
		if identifier == "" {
			identifier = p.Name
		}

		// Should we list packages without checksums?
		// Leaving this commented because it breaks with the kubernetes sboms
		// but perhaps we should be stricter here
		if len(p.Checksum) == 0 {
			logrus.Warn("SPDX package %s has no checksum", identifier)
			continue
		}

		artifact := run.Artifact{
			Path:     identifier,
			Checksum: map[string]string{},
		}
		for algo, c := range p.Checksum {
			artifact.Checksum[algo] = c
		}

		snap[identifier] = artifact
	}
	return &snap, nil
}
