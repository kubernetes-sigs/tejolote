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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDirectorySnap(t *testing.T) {
	// Create a fixed time to make the times deterministic
	fixedTime := time.Date(1976, time.Month(2), 10, 23, 30, 30, 0, time.Local)

	// Create some files in the directory
	for _, tc := range []struct {
		prepare func(path string) error
		mutate  func(path string) error
		expect  []Artifact
	}{
		// Two emtpy directories. No error, no change
		{
			func(path string) error { return nil },
			func(path string) error { return nil },
			[]Artifact{},
		},
		// One file, unchanged at mutation time
		{
			func(path string) error {
				return os.WriteFile(filepath.Join(path, "test.txt"), []byte("test"), os.FileMode(0o644))
			},
			func(path string) error { return nil },
			[]Artifact{},
		},
		// One file, rewritten should be reported
		{
			func(path string) error {
				return os.WriteFile(filepath.Join(path, "test.txt"), []byte("test"), os.FileMode(0o644))
			},
			func(path string) error {
				filePath := filepath.Join(path, "test.txt")
				if err := os.WriteFile(
					filePath, []byte("test"), os.FileMode(0o644),
				); err != nil {
					return err
				}
				if err := os.Chtimes(filePath, fixedTime, fixedTime); err != nil {
					return err
				}
				return nil
			},
			[]Artifact{
				{
					Path: "test.txt",
					Time: fixedTime,
					Hash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
				}},
		},
		// One file, with contents changed
		{
			func(path string) error {
				filePath := filepath.Join(path, "test.txt")
				if err := os.WriteFile(
					filePath, []byte("test"), os.FileMode(0o644),
				); err != nil {
					return err
				}
				if err := os.Chtimes(filePath, fixedTime, fixedTime); err != nil {
					return err
				}
				return nil
			},
			func(path string) error {
				filePath := filepath.Join(path, "test.txt")
				if err := os.WriteFile(
					filePath, []byte("test, but with a change!"), os.FileMode(0o644),
				); err != nil {
					return err
				}
				if err := os.Chtimes(filePath, fixedTime, fixedTime); err != nil {
					return err
				}
				return nil
			},
			[]Artifact{
				{
					Path: "test.txt",
					Time: fixedTime,
					Hash: "76aad9c1d52e424d0dd6c6b8e07169d5d5f9001a06fe5343d4bfa13c804788f0",
				}},
		},
	} {
		// Create a temp directory to operate in
		dir, err := os.MkdirTemp("", "")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		// Create the directory watcher
		sut := Directory{
			Path:      dir,
			Snapshots: []Snapshot{},
		}

		require.NoError(t, tc.prepare(dir))
		require.NoError(t, sut.Snap(), "creating first snapshot")

		require.NoError(t, tc.mutate(dir))
		require.NoError(t, sut.Snap(), "creating mutated fs snapshot")

		delta := sut.Snapshots[0].Delta(&sut.Snapshots[1])
		require.Equal(t, delta, tc.expect)
	}
}
