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

package sbom

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/puerco/tejolote/pkg/watcher"
	"sigs.k8s.io/bom/pkg/spdx"
	"sigs.k8s.io/release-utils/util"
)

type Parser struct {
	Options Options
}

type Options struct {
	CWD string
}

func (parser *Parser) ReadArtifacts(path string) (*[]watcher.Artifact, error) {
	doc, err := spdx.OpenDoc(path)
	if err != nil {
		return nil, fmt.Errorf("opening doc: %w", err)
	}

	list := []watcher.Artifact{}

	for _, p := range doc.Packages {
		artifactPath := filepath.Join(parser.Options.CWD, p.FileName)
		// Only add files if the file exists
		if !util.Exists(artifactPath) {
			continue
		}

		// Prefer sha256 to match

		list = append(list, watcher.Artifact{
			Path:     p.FileName,
			Checksum: p.Checksum,
			Time:     time.Time{},
		})
	}
	return &list, nil
}
