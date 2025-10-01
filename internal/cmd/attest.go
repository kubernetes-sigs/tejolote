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
	"sigs.k8s.io/release-utils/helpers"
	"sigs.k8s.io/tejolote/pkg/watcher"
)

type attestOptions struct {
	waitForBuild     bool
	sign             bool
	continueExisting string
	vcsurl           string // deprected
	dependencyURIs   []string
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
		Long: `
✨ tejolote attest buildsys://build-run/identifier
	
The attest subcommand queries a build system for information a about
a run. Generally this happens as part of a build, patching or cloning
repositories.

Tejolote will monitor for changes that occurred during the command
execution and will attest them to generate provenance data of where 
the changes originated.

🔸 Artifact Data
----------------

Tejolote supports a number of artifact data sources. You specify them
with the --artifact flag. For example, to read the artifacts uploaded
to a github release you specify the source URI as follows:

  --artifacts github://organization/repository/v1.0

This URI instructs tejolote to read the artifacts from the v1.0 release
page of the organization/repository repository. We support other sources
such as GCS buckets, OCI registries and so on. Check the docs for more
info.

🔸 Dependency Data
------------------

Tejolote will record the source build point as a dependency in the
SLSA predicate. If the build system reports more dependencies, these
will be added to the predicate as well.

You can manually add more dependencies using the --dependency flag:

  tejolote attest buildsys://build-run/identifier \
	--dependency=git+https://github.com/my/repo@shab81f221530e649... \
	--dependency=container/image@sha256:616dbcba501fa7a170871e0c3... \
	--dependency=https://example.com
	
As shown, you can add VCS locators image references that will get their
hashes properly recorded or other strings which will be treated as URIs
without interpreting them.

🔸 Waiting for Run Jobs
-----------------------

If the --wait flag is specified, tejolote will keep polling the build system
until the run is done. Once the build has concluded tejolote captures the
build data and generates the provenance attestation.

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
				return fmt.Errorf("building watcher: %w", err)
			}

			w.Builder.DependencyURIs = attestOpts.dependencyURIs
			if attestOpts.vcsurl != "" {
				w.Builder.DependencyURIs = append(w.Builder.DependencyURIs, attestOpts.vcsurl)
			}

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

			if helpers.Exists(outputOpts.FinalSnapshotStatePath(attestOpts.continueExisting)) {
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

	// vcs-url is now deprecated but keep it hidden for compat
	attestCmd.PersistentFlags().StringVar(
		&attestOpts.vcsurl,
		"vcs-url",
		"",
		"append a vcs URL to the attestation materials (deperected, use --dependency)",
	)
	_ = attestCmd.PersistentFlags().MarkHidden("vcs-url") //nolint:errcheck

	attestCmd.PersistentFlags().StringSliceVar(
		&attestOpts.dependencyURIs,
		"dependency",
		[]string{},
		"append a URI to the attestation dependencies (parses VCS locators and image refs)",
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
