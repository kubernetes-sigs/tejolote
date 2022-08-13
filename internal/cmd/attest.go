package cmd

import (
	"fmt"

	"github.com/puerco/tejolote/pkg/watcher"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type attestOptions struct {
	output    string
	artifacts []string
}

func addAttest(parentCmd *cobra.Command) {
	attestOpts := attestOptions{}
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

			r, err := w.GetRun(args[0])
			if err != nil {
				return fmt.Errorf("fetching run: %w", err)
			}

			if err := w.Watch(r); err != nil {
				return fmt.Errorf("generating attestation: %w", err)
			}

			for _, uri := range attestOpts.artifacts {
				if err := w.AddArtifactSource(uri); err != nil {
					return fmt.Errorf("adding artifacts source: %w", err)
				}
			}

			logrus.Infof("Run produced %d artifacts", len(r.Artifacts))

			attestation, err := r.Attest()
			if err != nil {
				return fmt.Errorf("generating run attestation: %w", err)
			}

			fmt.Println(attestation)
			return nil
		},
	}

	attestCmd.PersistentFlags().StringVar(
		&attestOpts.output,
		"output",
		"",
		"output file",
	)

	attestCmd.PersistentFlags().StringSliceVar(
		&attestOpts.artifacts,
		"artifacts",
		[]string{},
		"a storage URL to monitor for files",
	)

	parentCmd.AddCommand(attestCmd)
}
