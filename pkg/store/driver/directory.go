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
	"path/filepath"
	"strings"

	"sigs.k8s.io/release-utils/hash"

	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

func NewDirectory(specURL string) (*Directory, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing SpecURL %s: %w", specURL, err)
	}
	return &Directory{
		Path: u.Path,
	}, nil
}

type Directory struct {
	Path string
}

// Snap takes a snapshot of the directory
func (d *Directory) Snap() (*snapshot.Snapshot, error) {
	if d.Path == "" {
		return nil, fmt.Errorf("directory watcher has no path defined")
	}

	snap := snapshot.Snapshot{}

	// Walk the files in the directory
	if err := filepath.Walk(d.Path,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			// Hash the file
			sha, err := hash.SHA256ForFile(path)
			if err != nil {
				return fmt.Errorf("hashing %s: %w", path, err)
			}

			// Normalize the path....
			path, err = filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("normalizing path %s: %w", path, err)
			}

			// .. and trim the working directory to make it relative
			path = strings.TrimPrefix(path, d.Path+"/")

			// Register the file with the path normalized
			snap[path] = run.Artifact{
				Path:     path,
				Checksum: map[string]string{"SHA256": sha},
				Time:     info.ModTime(),
			}
			return nil
		}); err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	return &snap, nil
}
