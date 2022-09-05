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

package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

type outputOptions struct {
	OutputPath        string
	SnapshotStatePath string
	Workspace         string
}

// FinalSnapshotStatePath returns the final path to store/read the storage
// snapshots. The default mode is to store it by appending '.storage-snap.json'
// to the defaultSeed filename.
// It will always return a preset path in SnapshotStatePath
// A blank seed means do not store the data.
func (oo *outputOptions) FinalSnapshotStatePath(defaultSeed string) string {
	snapshotState := oo.SnapshotStatePath
	if oo.SnapshotStatePath == "default" {
		if defaultSeed == "" {
			return ""
		}
		snapshotState = strings.TrimSuffix(defaultSeed, ".json") + ".storage-snap.json"
	}
	return snapshotState
}

func addOutputFlags(command *cobra.Command) *outputOptions {
	opts := &outputOptions{}
	command.PersistentFlags().StringVar(
		&opts.OutputPath,
		"output",
		"",
		"file to store the partial attestation (instead of STDOUT)",
	)
	command.PersistentFlags().StringVar(
		&opts.SnapshotStatePath,
		"snapshots",
		"default",
		"path to store the storage snapshots state",
	)
	return opts
}
