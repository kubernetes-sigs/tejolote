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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"sigs.k8s.io/yaml"
)

var ghContentsURL = "https://api.github.com/repos/%s/%s/contents/%s?ref=%s"

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

// workflowFile is a minimal representation of a GitHub Actions workflow YAML.
// Note: in YAML, "on" is a boolean keyword that gets converted to "true" by
// sigs.k8s.io/yaml's YAML-to-JSON conversion, so we use json:"true" here.
type workflowFile struct {
	On workflowTrigger `json:"true"`
}

// contentsResponse represents the GitHub contents API response.
type contentsResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// FetchWorkflowInputs fetches the workflow YAML from the GitHub contents API
// and returns the defined inputs (from workflow_dispatch and workflow_call triggers).
func FetchWorkflowInputs(org, repo, path, ref string) (map[string]WorkflowInput, error) {
	apiURL := fmt.Sprintf(ghContentsURL, org, repo, url.PathEscape(path), url.QueryEscape(ref))

	res, err := APIGetRequest(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching workflow file: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got HTTP %d fetching workflow file", res.StatusCode)
	}

	rawData, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading workflow contents response: %w", err)
	}

	var cr contentsResponse
	if err := json.Unmarshal(rawData, &cr); err != nil {
		return nil, fmt.Errorf("unmarshalling contents response: %w", err)
	}

	if cr.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected content encoding %q", cr.Encoding)
	}

	yamlData, err := base64.StdEncoding.DecodeString(cr.Content)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 content: %w", err)
	}

	var wf workflowFile
	if err := yaml.Unmarshal(yamlData, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}

	inputs := map[string]WorkflowInput{}
	if wf.On.WorkflowDispatch != nil {
		for k, v := range wf.On.WorkflowDispatch.Inputs {
			inputs[k] = v
		}
	}
	if wf.On.WorkflowCall != nil {
		for k, v := range wf.On.WorkflowCall.Inputs {
			inputs[k] = v
		}
	}

	return inputs, nil
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
