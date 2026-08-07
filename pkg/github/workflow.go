/*
Copyright 2026 The Kubernetes Authors.

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

package github

import (
	"context"
	"fmt"

	gogithub "github.com/google/go-github/v90/github"
	"sigs.k8s.io/yaml"
)

// WorkflowInput represents a single input defined in a workflow YAML.
type WorkflowInput struct {
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Type        string `json:"type"`
}

// workflowTrigger represents the "on" section of a workflow YAML.
type workflowTrigger struct {
	WorkflowDispatch *workflowTriggerInputs `json:"workflow_dispatch"`
	WorkflowCall     *workflowTriggerInputs `json:"workflow_call"`
}

type workflowTriggerInputs struct {
	Inputs map[string]WorkflowInput `json:"inputs"`
}

// WorkflowJob represents a job definition in a workflow YAML file.
type WorkflowJob struct {
	Uses string `json:"uses"` // Non-empty when job is a reusable workflow call
}

// WorkflowData is a parsed representation of a GitHub Actions workflow YAML.
// Note: in YAML, "on" is a boolean keyword that gets converted to "true" by
// sigs.k8s.io/yaml's YAML-to-JSON conversion, so we use json:"true" here.
type WorkflowData struct {
	On   workflowTrigger        `json:"true"`
	Jobs map[string]WorkflowJob `json:"jobs"`
}

// Inputs returns the defined inputs from workflow_dispatch and workflow_call
// triggers, merged into a single map.
func (wd *WorkflowData) Inputs() map[string]WorkflowInput {
	inputs := map[string]WorkflowInput{}
	if wd.On.WorkflowDispatch != nil {
		for k, v := range wd.On.WorkflowDispatch.Inputs {
			inputs[k] = v
		}
	}
	if wd.On.WorkflowCall != nil {
		for k, v := range wd.On.WorkflowCall.Inputs {
			inputs[k] = v
		}
	}
	return inputs
}

// JobKeys returns the YAML keys of all jobs defined in the workflow.
func (wd *WorkflowData) JobKeys() []string {
	keys := make([]string, 0, len(wd.Jobs))
	for k := range wd.Jobs {
		keys = append(keys, k)
	}
	return keys
}

// FetchWorkflow fetches and parses a workflow YAML from the GitHub contents API.
func FetchWorkflow(org, repo, path, ref string) (*WorkflowData, error) {
	client, err := defaultClient()
	if err != nil {
		return nil, fmt.Errorf("creating github client: %w", err)
	}

	fileContent, _, _, err := client.Repositories.GetContents(
		context.Background(), org, repo, path,
		&gogithub.RepositoryContentGetOptions{Ref: ref},
	)
	if err != nil {
		return nil, fmt.Errorf("fetching workflow file: %w", err)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("%s is not a file in %s/%s", path, org, repo)
	}

	yamlData, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("decoding workflow contents: %w", err)
	}

	var wf WorkflowData
	if err := yaml.Unmarshal([]byte(yamlData), &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	return &wf, nil
}

// FetchWorkflowInputs fetches the workflow YAML and returns the defined inputs.
// This is a convenience wrapper around FetchWorkflow for callers that only need inputs.
func FetchWorkflowInputs(org, repo, path, ref string) (map[string]WorkflowInput, error) {
	wf, err := FetchWorkflow(org, repo, path, ref)
	if err != nil {
		return nil, err
	}
	return wf.Inputs(), nil
}

// EffectiveInputs computes the effective input values by merging actual run
// inputs with the defaults defined in the workflow YAML. Run values take
// precedence over defaults.
func EffectiveInputs(defined map[string]WorkflowInput, runInputs map[string]string) map[string]string {
	result := make(map[string]string, len(defined))
	for name, def := range defined {
		if val, ok := runInputs[name]; ok {
			result[name] = val
		} else if def.Default != "" {
			result[name] = def.Default
		}
	}
	return result
}
