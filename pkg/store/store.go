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

package store

import (
	"fmt"
	"net/url"
	"strings"

	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/driver"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type Store struct {
	SpecURL string
	Driver  Implementation
}

type Implementation interface {
	Snap() (*snapshot.Snapshot, error)
}

func New(specURL string) (s Store, err error) {
	s = Store{}
	u, err := url.Parse(specURL)
	if err != nil {
		return s, fmt.Errorf("parsing storage spec URL %s: %w", specURL, err)
	}
	var impl Implementation
	switch u.Scheme {
	case "file":
		impl, err = driver.NewDirectory(specURL)
	case "gs":
		impl, err = driver.NewGCS(specURL)
	case "oci":
		impl, err = driver.NewOCI(specURL)
	case "actions":
		impl, err = driver.NewActions(specURL)
	case "gcb":
		impl, err = driver.NewGCB(specURL)
	case "github":
		impl, err = driver.NewGithub(specURL)
	default:
		// Attestation use a composed scheme
		format, _, ok := strings.Cut(u.Scheme, "+")
		if !ok {
			return s, fmt.Errorf("%s is not a storage URL", specURL)
		}
		switch format {
		case "intoto":
			impl, err = driver.NewAttestation(specURL)
		case "spdx":
			impl, err = driver.NewSPDX(specURL)
		default:
			err = fmt.Errorf("unknown storage backend %s", format)
		}
	}
	if err != nil {
		return s, fmt.Errorf("initializing storage backend: %w", err)
	}
	s.SpecURL = specURL
	s.Driver = impl

	return s, nil
}

// ReadArtifacts returns the combined list of artifacts from
// every store attached to the watcher
func (s *Store) ReadArtifacts() ([]run.Artifact, error) {
	artifacts := []run.Artifact{}
	snap, err := s.Driver.Snap()
	if err != nil {
		return artifacts, fmt.Errorf("snapshotting storage: %w", err)
	}
	for _, a := range *snap {
		artifacts = append(artifacts, a)
	}
	return artifacts, nil
}

// Snap calls the underlying driver's Snap method to capture
// the current store's state into a snapshot
func (s *Store) Snap() (*snapshot.Snapshot, error) {
	return s.Driver.Snap()
}
