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

package driver

import (
	"fmt"
	"net/url"

	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store"
)

const (
	GITHUB = "github"
)

// BuildSystemDriver is an interface to a type that can query a buildsystem
// for data required to build a provenance attestation
type BuildSystem interface {
	GetRun(string) (*run.Run, error)
	RefreshRun(*run.Run) error
	BuildPredicate(*run.Run, attestation.Predicate) (attestation.Predicate, error)
	ArtifactStores() []store.Store
}

func NewFromSpecURL(specURL string) (BuildSystem, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing run spec URL: %w", err)
	}

	var driver BuildSystem
	switch u.Scheme {
	case "gcb":
		driver, err = NewGCB(specURL)
		if err != nil {
			return nil, fmt.Errorf("creating GCB driver: %w", err)
		}
	case GITHUB:
		driver = &GitHubWorkflow{}
	default:
		return nil, fmt.Errorf("unable to get driver from url %s", specURL)
	}
	return driver, nil
}

func NewFromMoniker(moniker string) (BuildSystem, error) {
	var driver BuildSystem
	switch moniker {
	case "gcb":
		driver = &GCB{}
	case GITHUB:
		driver = &GitHubWorkflow{}
	default:
		return nil, fmt.Errorf("unable to get driver from moniker %s", moniker)
	}
	return driver, nil
}
