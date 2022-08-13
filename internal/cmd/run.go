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
	gexec "os/exec"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/puerco/tejolote/pkg/exec"
	"github.com/puerco/tejolote/pkg/run"
)

type runOptions struct {
	Verbose    bool
	CWD        string
	OutputDirs []string
}

func addRun(parentCmd *cobra.Command) {
	runOpts := runOptions{}
	runCmd := &cobra.Command{
		Short: "Execute one or more builder steps",
		Long: `tejolote run [command]
	
The run subcommand os tejolote executes a process intended to
transform files. Generally this happens as part of a build, patching
or cloning repositories.

Tejolote will monitor for changes that occurred during the command
execution and will attest to them to generate provenance data of
where they came from.
	
	`,
		Use:               "run",
		SilenceUsage:      false,
		PersistentPreRunE: initLogging,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var runner *exec.Runner
			runner, err = buildRunner(runOpts)
			if err != nil {
				return fmt.Errorf("creating runner: %w", err)
			}

			var step *run.Step
			if len(args) > 0 {
				step, err = syntheticStepFromArgs(args...)
				if err != nil {
					return fmt.Errorf("generating step from arguments: %w", err)
				}
			}

			if step == nil {
				logrus.Warn("💣 Error. Nothing to execute.")
				logrus.Warn("Define something to run in the command line or define one or more steps")
				logrus.Warn("in a configuration file.")

				return errors.New("no step to run")
			}

			// What do we do with the run?
			run, err2 := runner.RunStep(*step)
			if err2 != nil {
				return fmt.Errorf("executing step: %w", err)
			}

			logrus.Infof("Run produced %d artifacts", len(run.Artifacts))
			return nil
		},
	}

	runCmd.PersistentFlags().StringSliceVar(
		&runOpts.OutputDirs,
		"dir",
		[]string{"."},
		"list of directories that tejolote will monitor for output",
	)

	runCmd.PersistentFlags().StringVarP(
		&runOpts.CWD,
		"cwd",
		"C",
		"",
		"directory to change when running the build",
	)

	runCmd.PersistentFlags().BoolVar(
		&runOpts.Verbose,
		"verbose",
		false,
		"verbose output (prints commands and output)",
	)

	parentCmd.AddCommand(runCmd)
}

// buildRunner returns a configured runner
func buildRunner(opts runOptions) (*exec.Runner, error) {
	runner := exec.NewRunner()
	runner.Options.CWD = opts.CWD

	/*
		for _, dir := range opts.OutputDirs {
			store, err := store.New(dir)
			logrus.Infof("Watching directory: %s", dir)
			runner.Watchers = append(runner.Watchers, store)
		}
	*/

	return runner, nil
}

// syntheticStepFromArgs evaluates the arguments passed to see if
// they correspond to an executable which may be contrued into
// a tejolote run
func syntheticStepFromArgs(args ...string) (*run.Step, error) {
	if len(args) == 0 {
		return nil, errors.New("no arguments ")
	}

	// Check for executable
	if _, err := gexec.LookPath(args[0]); err != nil {
		return nil, fmt.Errorf("executable '%s' not found", args[0])
	}

	params := []string{}
	if len(args) > 1 {
		params = args[1:]
	}

	step := run.Step{
		Command:     args[0],
		IsSuccess:   false,
		Params:      params,
		StartTime:   time.Time{},
		EndTime:     time.Time{},
		Environment: map[string]string{},
	}

	return &step, nil
}
