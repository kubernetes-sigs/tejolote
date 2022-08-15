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
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"chainguard.dev/apko/pkg/vcs"
	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"

	"github.com/puerco/tejolote/pkg/attestation"
	"github.com/spf13/cobra"
)

type startAttestationOptions struct {
	outputOptions
	clone     bool
	repo      string
	repoPath  string
	workspace string
}

func (opts startAttestationOptions) Validate() error {
	if opts.clone && opts.repo == "" {
		return errors.New("repository clone requested but no repository was specified")
	}

	if opts.clone && opts.repoPath == "" {
		return errors.New("repository clone requested but no repository path was specified")
	}
	return nil
}

func addStart(parentCmd *cobra.Command) {
	startAttestationOpts := startAttestationOptions{}

	// Verb
	startCmd := &cobra.Command{
		Short:             "Start a partial document",
		Use:               "start",
		SilenceUsage:      false,
		PersistentPreRunE: initLogging,
	}

	// Noun
	startAttestationCmd := &cobra.Command{
		Short: "Attest to a build system run",
		Long: `tejolote start attestation
	
The start command of tejolte writes a partial attestation 
containing initial data that can be observed before launching a
build. The partial attestation is meant to be completed by
tejolote once it finished observing a build.
	
	`,
		Use:               "attestation",
		SilenceUsage:      false,
		PersistentPreRunE: initLogging,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err := startAttestationOpts.Validate(); err != nil {
				return fmt.Errorf("validating options: %w", err)
			}
			att := attestation.New()
			predicate := attestation.NewSLSAPredicate()

			if startAttestationOpts.clone {
				// TODO: Implement
				return fmt.Errorf("repository cloning not yet implemented")
			}

			vcsURL, err := readVCSURL(startAttestationOpts)
			if err != nil {
				return fmt.Errorf("fetching VCS URL: %w", err)
			}

			if vcsURL != "" {
				material := slsa.ProvenanceMaterial{
					URI:    vcsURL,
					Digest: map[string]string{},
				}
				if repoURL, repoDigest, ok := strings.Cut(vcsURL, "@"); ok {
					material.URI = repoURL
					material.Digest["sha1"] = repoDigest
				}
				predicate.Materials = append(predicate.Materials, material)
			}

			att.Predicate = predicate

			json, err := att.ToJSON()
			if err != nil {
				return fmt.Errorf("serializing attestation json: %w", err)
			}
			fmt.Println(string(json))

			return nil
		},
	}

	startAttestationCmd.PersistentFlags().StringVar(
		&startAttestationOpts.repo,
		"repository",
		"",
		"url of repository containing the main project source",
	)

	startAttestationCmd.PersistentFlags().StringVar(
		&startAttestationOpts.repoPath,
		"repo-path",
		".",
		"path to the main code repository (relative to workspace)",
	)

	startAttestationCmd.PersistentFlags().StringVar(
		&startAttestationOpts.workspace,
		"workspace",
		"",
		"path to the workspace where the build runs",
	)

	startAttestationCmd.PersistentFlags().BoolVar(
		&startAttestationOpts.clone,
		"clone",
		false,
		"clone the repository",
	)

	startAttestationOpts.outputOptions = addOutputFlags(startAttestationCmd)
	startCmd.AddCommand(startAttestationCmd)
	parentCmd.AddCommand(startCmd)
}

// readVCSURL checks the repository path to get the VCS url for the
// materials
func readVCSURL(opts startAttestationOptions) (string, error) {
	if opts.repoPath == "" {
		return "", nil
	}

	repoPath := opts.repoPath

	// If its a relative URL, append the workspace
	if !strings.HasPrefix(opts.repoPath, string(filepath.Separator)) {
		repoPath = filepath.Join(opts.workspace, opts.repoPath)
	}

	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path to repo: %w", err)
	}

	urlString, err := vcs.ProbeDirForVCSUrl(repoPath, repoPath)
	if err != nil {
		return "", fmt.Errorf("probing VCS URL: %w", err)
	}
	return urlString, nil
}
