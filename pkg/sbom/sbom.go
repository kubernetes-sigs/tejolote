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

package sbom

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/protobom/protobom/pkg/reader"
	"github.com/protobom/protobom/pkg/sbom"
	"sigs.k8s.io/release-utils/helpers"
	"sigs.k8s.io/tejolote/pkg/run"
)

type Parser struct {
	Options Options
}

type Options struct {
	CWD        string
	CheckPaths bool
}

// ReadArtifacts reads the artifact list from an SBOM
func (parser *Parser) ReadArtifacts(path string) (*[]run.Artifact, error) {
	r := reader.New()
	doc, err := r.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parsing SBOM from %q: %w", path, err)
	}

	list := []run.Artifact{}

	// Return the top level nodes, avoiding dependencies. This probably shoould
	// be more flexible but most SBOMs are structured this way.
	for _, n := range doc.GetRootNodes() {
		// Only add files if the file exists
		if parser.Options.CheckPaths {
			if n.GetFileName() == "" {
				continue
			}
			artifactPath := filepath.Join(parser.Options.CWD, n.GetFileName())
			if !helpers.Exists(artifactPath) {
				continue
			}
		}

		// Prefer sha256 to match
		artifact := run.Artifact{
			Path:     n.GetFileName(),
			Checksum: map[string]string{},
			Time:     time.Time{},
		}

		for algoID, value := range n.GetHashes() {
			artifact.Checksum[sbom.HashAlgorithm(algoID).String()] = value
		}

		list = append(list, artifact)
	}
	return &list, nil
}
