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

package cmd

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/release-utils/util"
	"sigs.k8s.io/tejolote/pkg/watcher"
)

type attestOptions struct {
	waitForBuild     bool
	sign             bool
	continueExisting string
	vcsurl           string
	encodedExisting  string
	encodedSnapshots string
	slsaVersion      string
	artifacts        []string
}

var slsaVersions = []string{"1", "1.0", "0.2"}

func (o *attestOptions) Verify() error {
	errs := []error{}
	if o.encodedExisting != "" && o.continueExisting != "" {
		errs = append(errs, errors.New("only --encoded-existing or --continue can be set at a time"))
	}

	if !slices.Contains(slsaVersions, o.slsaVersion) {
		errs = append(errs, fmt.Errorf("invalid slsa versions must be one of %v", slsaVersions))
	}
	return errors.Join(errs...)
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
		RunE: func(_ *cobra.Command, args []string) (err error) {
			if len(args) == 0 {
				return errors.New("build run spec URL not specified")
			}

			if err := attestOpts.Verify(); err != nil {
				return fmt.Errorf("verifying options: %w", err)
			}

			w, err := watcher.New(args[0])
			if err != nil {
				return fmt.Errorf("building watcher")
			}

			w.Builder.VCSURL = attestOpts.vcsurl

			w.Options.WaitForBuild = attestOpts.waitForBuild
			w.Options.SLSAVersion = attestOpts.slsaVersion

			if !attestOpts.waitForBuild {
				logrus.Warn("watcher will not wait for build, data may be incomplete")
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
				return fmt.Errorf("waiting for the run to finish: %w", err)
			}

			if attestOpts.encodedExisting != "" {
				f, err := os.CreateTemp("", "attestation-*.intoto.json")
				if err != nil {
					return fmt.Errorf("marshallling encoded attestation: %w", err)
				}
				defer f.Close()
				decodedAtt, err := base64.StdEncoding.DecodeString(attestOpts.encodedExisting)
				if err != nil {
					return fmt.Errorf("decoding existing attestation")
				}
				if err := os.WriteFile(f.Name(), decodedAtt, os.FileMode(0o644)); err != nil {
					return fmt.Errorf("writing encoded attestation to disk")
				}
				attestOpts.continueExisting = f.Name()
			}

			if attestOpts.encodedSnapshots != "" {
				f, err := os.CreateTemp("", "snapshots-*.intoto.json")
				if err != nil {
					return fmt.Errorf("marshallling encoded snapshots: %w", err)
				}
				defer f.Close()
				decodedSnaps, err := base64.StdEncoding.DecodeString(attestOpts.encodedSnapshots)
				if err != nil {
					return fmt.Errorf("decoding received snapshots: %w", err)
				}
				if err := os.WriteFile(f.Name(), decodedSnaps, os.FileMode(0o644)); err != nil {
					return fmt.Errorf("writing encoded attestation to disk")
				}
				outputOpts.SnapshotStatePath = f.Name()
			}

			if err = w.LoadAttestation(attestOpts.continueExisting); err != nil {
				return fmt.Errorf("loading previous attestation")
			}

			if util.Exists(outputOpts.FinalSnapshotStatePath(attestOpts.continueExisting)) {
				if err := w.LoadSnapshots(
					outputOpts.FinalSnapshotStatePath(attestOpts.continueExisting),
				); err != nil {
					return fmt.Errorf("loading storage snapshots: %w", err)
				}
			}

			if err := w.CollectArtifacts(r); err != nil {
				return fmt.Errorf("while collecting run artifacts: %w", err)
			}

			attestation, err := w.AttestRun(r)
			if err != nil {
				return fmt.Errorf("generating run attestation: %w", err)
			}

			var json []byte

			if attestOpts.sign {
				json, err = attestation.Sign()
			} else {
				json, err = attestation.ToJSON()
			}

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

	attestCmd.PersistentFlags().BoolVar(
		&attestOpts.sign,
		"sign",
		false,
		"sign the attestation",
	)

	attestCmd.PersistentFlags().StringSliceVar(
		&attestOpts.artifacts,
		"artifacts",
		[]string{},
		"a storage URL to monitor for files",
	)
	attestCmd.PersistentFlags().BoolVar(
		&attestOpts.waitForBuild,
		"wait",
		true,
		"when watrching the run, wait for the build to finish",
	)
	attestCmd.PersistentFlags().StringVar(
		&attestOpts.vcsurl,
		"vcs-url",
		"",
		"append a vcs URL to the atetstation materials",
	)
	attestCmd.PersistentFlags().StringVar(
		&attestOpts.encodedExisting,
		"encoded-attestation",
		"",
		"encoded attestation to continue",
	)
	attestCmd.PersistentFlags().StringVar(
		&attestOpts.encodedSnapshots,
		"encoded-snapshots",
		"",
		"encoded snapshots to continue",
	)

	attestCmd.PersistentFlags().StringVar(
		&attestOpts.slsaVersion,
		"slsa",
		"1.0",
		fmt.Sprintf("SLSA attestation version %v", slsaVersions),
	)

	_ = attestCmd.PersistentFlags().MarkHidden("encoded-attestation")
	_ = attestCmd.PersistentFlags().MarkHidden("encoded-snapshots")

	parentCmd.AddCommand(attestCmd)
}
