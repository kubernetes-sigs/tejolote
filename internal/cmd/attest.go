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
	"fmt"
	"os"

	"github.com/puerco/tejolote/pkg/watcher"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type attestOptions struct {
	continueExisting string
	artifacts        []string
}

func addAttest(parentCmd *cobra.Command) {
	attestOpts := attestOptions{}
	var outputOpts *outputOptions

	attestCmd := &cobra.Command{
		Short: "Attest to a build system run",
		Long: `tejolote attest buildsys://build-run/identifier
	
The run subcommand os tejolote executes a process intended to
transform files. Generally this happens as part of a build, patching
or cloning repositories.

Tejolote will monitor for changes that occurred during the command
execution and will attest to them to generate provenance data of
where they came from.
	
	`,
		Use:               "attest",
		SilenceUsage:      false,
		PersistentPreRunE: initLogging,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			w, err := watcher.New(args[0])
			if err != nil {
				return fmt.Errorf("building watcher")
			}

			// Add artifact monitors to the watcher
			for _, uri := range attestOpts.artifacts {
				if err := w.AddArtifactSource(uri); err != nil {
					return fmt.Errorf("adding artifacts source: %w", err)
				}
			}

			// Get the run from the build system
			r, err := w.GetRun(args[0])
			if err != nil {
				return fmt.Errorf("fetching run: %w", err)
			}

			// Watch the run run :)
			if err := w.Watch(r); err != nil {
				return fmt.Errorf("generating attestation: %w", err)
			}

			logrus.Infof("Run produced %d artifacts", len(r.Artifacts))

			if w.LoadAttestation(attestOpts.continueExisting); err != nil {
				return fmt.Errorf("loading previous attestation")
			}

			if err := w.CollectArtifacts(r); err != nil {
				return fmt.Errorf("while collecting run artifacts: %w", err)
			}

			attestation, err := w.AttestRun(r)
			if err != nil {
				return fmt.Errorf("generating run attestation: %w", err)
			}

			json, err := attestation.ToJSON()
			if err != nil {
				return fmt.Errorf("serializing attestation: %w", err)
			}

			if outputOpts.OutputPath != "" {
				if err := os.WriteFile(outputOpts.OutputPath, json, os.FileMode(0o644)); err != nil {
					return fmt.Errorf("writing attestation file: %w", err)
				}
				return nil
			}

			fmt.Println(string(json))
			return nil
		},
	}

	outputOpts = addOutputFlags(attestCmd)

	attestCmd.PersistentFlags().StringVar(
		&attestOpts.continueExisting,
		"continue",
		"",
		"path to a previously started attestation to continue",
	)

	attestCmd.PersistentFlags().StringSliceVar(
		&attestOpts.artifacts,
		"artifacts",
		[]string{},
		"a storage URL to monitor for files",
	)

	parentCmd.AddCommand(attestCmd)
}
