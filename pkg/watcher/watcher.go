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

package watcher

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	intoto "github.com/in-toto/in-toto-golang/in_toto"
	slsa "github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/v0.2"

	"github.com/puerco/tejolote/pkg/attestation"
	"github.com/puerco/tejolote/pkg/builder"
	"github.com/puerco/tejolote/pkg/run"
	"github.com/puerco/tejolote/pkg/store"
	"github.com/sirupsen/logrus"
)

type Watcher struct {
	DraftAttestation *attestation.Attestation
	Builder          builder.Builder
	ArtifactStores   []store.Store
}

func New(uri string) (w *Watcher, err error) {
	w = &Watcher{}

	// Get the builder
	b, err := builder.New(uri)
	if err != nil {
		return nil, fmt.Errorf("getting build watcher: %w", err)
	}
	w.Builder = b

	return w, nil
}

// GetRun returns a run from the build system
func (w *Watcher) GetRun(specURL string) (*run.Run, error) {
	r, err := w.Builder.GetRun(specURL)
	if err != nil {
		return nil, fmt.Errorf("getting run: %w", err)
	}
	return r, nil
}

// Watch watches a run, updating the run data as it runs
func (w *Watcher) Watch(r *run.Run) error {
	for {
		if !r.IsRunning {
			return nil
		}

		// Sleep to wait for a status change
		if err := w.Builder.RefreshRun(r); err != nil {
			return fmt.Errorf("refreshing run data: %w", err)
		}
		// Sleep
		time.Sleep(3 * time.Second)
	}
}

// LoadAttestation loads a partial attestation to complete
// when a run finished running
func (w *Watcher) LoadAttestation(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("loading previous attestation: %w", err)
	}

	att := attestation.New().SLSA()

	if err := json.Unmarshal(data, &att); err != nil {
		return fmt.Errorf("unmarshaling attestation json: %w", err)
	}

	w.DraftAttestation = att
	logrus.Infof("Loaded draft attestation from %s", path)
	return nil
}

// AttestRun generates an attestation from a run tejolote can watch
func (w *Watcher) AttestRun(r *run.Run) (att *attestation.Attestation, err error) {
	if r.IsRunning {
		logrus.Warn("run is still running, attestation may not capture en result")
	}

	if w.DraftAttestation != nil {
		att = w.DraftAttestation
	} else {
		att = attestation.New().SLSA()
	}

	var pred *attestation.SLSAPredicate
	if p, ok := att.Predicate.(attestation.SLSAPredicate); ok {
		pred = &p
	}

	predicate, err := w.Builder.BuildPredicate(r, pred)
	if err != nil {
		return nil, fmt.Errorf("building predicate: %w", err)
	}

	// Add the run artifacts to the attestation
	for _, a := range r.Artifacts {
		s := intoto.Subject{
			Name:   a.Path,
			Digest: slsa.DigestSet{},
		}
		for a, v := range a.Checksum {
			s.Digest[a] = v
		}
		att.Subject = append(att.Subject, s)
	}

	att.Predicate = predicate
	return att, nil
}

// AddArtifactSource adds a new source to look for artifacts
func (w *Watcher) AddArtifactSource(specURL string) error {
	s, err := store.New(specURL)
	if err != nil {
		return fmt.Errorf("getting artifact store: %w", err)
	}
	w.ArtifactStores = append(w.ArtifactStores, s)
	return nil
}

// CollectArtifacts queries the storage drivers attached to the run and
// collects any artifacts found after the build is done
func (w *Watcher) CollectArtifacts(r *run.Run) error {
	r.Artifacts = nil
	for _, s := range w.ArtifactStores {
		logrus.Infof("Collecting artifacts from %s", s.SpecURL)
		artifacts, err := s.ReadArtifacts()
		if err != nil {
			return fmt.Errorf("collecting artfiacts from %s: %w", s.SpecURL, err)
		}
		r.Artifacts = append(r.Artifacts, artifacts...)
	}
	logrus.Info(
		"Run produced %d artifacts collected from %d sources",
		len(r.Artifacts), len(w.ArtifactStores),
	)
	return nil
}
