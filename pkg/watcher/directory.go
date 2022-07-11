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

package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/release-utils/hash"
)

func NewDirectory(path string) *Directory {
	return &Directory{
		Path:      path,
		Snapshots: []Snapshot{},
	}
}

type Directory struct {
	Path      string
	Snapshots []Snapshot
}

// Snap takes a snapshot of the directory
func (d *Directory) Snap() error {
	if d.Path == "" {
		return errors.New("directory watcher has no path defined")
	}

	snap := Snapshot{}

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
			snap[path] = Artifact{
				Path: path,
				Hash: sha,
				Time: info.ModTime(),
			}
			return nil
		}); err != nil {
		return fmt.Errorf("walking directory: %w", err)
	}

	if d.Snapshots == nil {
		d.Snapshots = []Snapshot{}
	}
	d.Snapshots = append(d.Snapshots, snap)

	return nil
}
