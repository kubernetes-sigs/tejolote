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

package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	intoto "github.com/in-toto/in-toto-golang/in_toto"
	"github.com/in-toto/in-toto-golang/in_toto/slsa_provenance/common"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/builder"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type Watcher struct {
	DraftAttestation *attestation.Attestation
	Builder          builder.Builder
	ArtifactStores   []store.Store
	Snapshots        []map[string]*snapshot.Snapshot
	Options          Options
}

type Options struct {
	WaitForBuild bool   // When true, the watcher will keep observing the run until it's done
	SLSAVersion  string // SLSA version for the attestation predicate
}

func New(uri string) (w *Watcher, err error) {
	w = &Watcher{
		Options: Options{
			WaitForBuild: true, // By default we watch the build run
		},
	}

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

		if !w.Options.WaitForBuild {
			logrus.Warn("run is still running but watcher won't wait (WaitForBuild = false)")
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
	switch w.Options.SLSAVersion {
	case "1", "1.0":
		att = att.SLSAv1()
	case "0.2", "":
		att = att.SLSA()
	default:
		return fmt.Errorf("invalid SLSA version")
	}

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

	// Generate the attestation according to the required version
	att = attestation.New()
	switch w.Options.SLSAVersion {
	case "1", "1.0":
		att = att.SLSAv1()
	case "0.2", "":
		att = att.SLSA()
	default:
		return nil, fmt.Errorf("invalid SLSA version")
	}

	if w.DraftAttestation != nil {
		att = w.DraftAttestation
	}

	// Here, we need to check if its empty
	pred := att.Predicate
	predicate, err := w.Builder.BuildPredicate(r, pred)
	if err != nil {
		return nil, fmt.Errorf("building predicate: %w", err)
	}

	// Add the run artifacts to the attestation
	for _, a := range r.Artifacts {
		s := intoto.Subject{
			Name:   a.Path,
			Digest: common.DigestSet{},
		}
		maps.Copy(s.Digest, a.Checksum)
		att.Subject = append(att.Subject, s)
	}

	att.Predicate = predicate
	att.PredicateType = att.Predicate.Type()
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
	artifactStores := w.ArtifactStores
	// TODO: Support disabling the native driver
	artifactStores = append(artifactStores, w.Builder.ArtifactStores()...)
	for _, s := range artifactStores {
		logrus.Infof("Collecting artifacts from %s", s.SpecURL)
		artifacts, err := s.ReadArtifacts()
		if err != nil {
			return fmt.Errorf("collecting artfiacts from %s: %w", s.SpecURL, err)
		}
		r.Artifacts = append(r.Artifacts, artifacts...)
	}
	logrus.Infof(
		"Run produced %d artifacts collected from %d sources",
		len(r.Artifacts), len(w.ArtifactStores),
	)
	return nil
}

// Snap adds a new snapshot set to the watcher by querying
// each of the storage drivers
func (w *Watcher) Snap() error {
	snaps := map[string]*snapshot.Snapshot{}
	for _, s := range w.ArtifactStores {
		if s.SpecURL == "" {
			return errors.New("artifact store has no spec url defined")
		}
		snap, err := s.Snap()
		if err != nil {
			return fmt.Errorf("snapshotting storage: %w", err)
		}
		snaps[s.SpecURL] = snap
	}
	// TODO: Add some metrics to measure snapshot time
	w.Snapshots = append(w.Snapshots, snaps)
	return nil
}

// SaveSnapshots stores the current state of the storage locations
// to a file which can be reused when continuing an attestation
func (w *Watcher) SaveSnapshots(path string) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if len(w.Snapshots) == 0 {
		logrus.Debug("no storage snapshots set, not saving file")
		return nil
	}
	if err := enc.Encode(w.Snapshots); err != nil {
		return fmt.Errorf("encoding snapshot data sbom: %w", err)
	}

	if err := os.WriteFile(path, b.Bytes(), os.FileMode(0o644)); err != nil {
		return fmt.Errorf("writing file store state: %w", err)
	}
	return nil
}

// LoadSnapshots loads saved snapshot state from a file to continue
func (w *Watcher) LoadSnapshots(path string) error {
	if path == "" {
		return nil
	}
	rawData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("opening saved snapshot data: %w", err)
	}
	snapData := []map[string]*snapshot.Snapshot{}
	if err := json.Unmarshal(rawData, &snapData); err != nil {
		return fmt.Errorf("unmarshaling snapshot data: %w", err)
	}

	// Check the loaded snapshots
	for i, snapset := range snapData {
		if err := w.checkSnapshotMatch(snapset); err != nil {
			return fmt.Errorf("checking restored storage state #%d: %w", i, err)
		}
	}
	w.Snapshots = snapData
	logrus.Infof("loaded %d snapshot sets from %s", len(w.Snapshots), path)

	return nil
}

// checkSnapshotMatch checks that a snapshot set matches the configured
// storage backends in the watcher. The snapshots need to match in order
// and in the SpecURL
func (w *Watcher) checkSnapshotMatch(snapset map[string]*snapshot.Snapshot) error {
	if len(snapset) != len(w.ArtifactStores) {
		return fmt.Errorf(
			"the number of artifact stores in the watcher (%d) does not match the number in the stored set (%d)",
			len(w.ArtifactStores), len(snapset),
		)
	}

	// Check that the SpecURLs match those in the configured stores:
	i := 0
	for specurl := range snapset {
		if w.ArtifactStores[i].SpecURL != specurl {
			return fmt.Errorf(
				"spec url #%d in stored state, does not match storage %s",
				i, w.ArtifactStores[i].SpecURL,
			)
		}
		i++
	}
	return nil
}

type StartMessage struct {
	SpecURL      string   `json:"spec"`
	Attestation  string   `json:"attestation"`
	Snapshots    string   `json:"snapshots"`
	ArtifactList string   `json:"artifacts_list"`
	Artifacts    []string `json:"artifacts"`
}

// PublishToTopic sends the data of a partial attestation to a Pub/Sub
// topic.
func (w *Watcher) PublishToTopic(topicString string, message interface{}) (err error) {
	// projects/puerco-chainguard/topics/slsa
	parts := strings.Split(topicString, "/")
	if len(parts) != 4 {
		return errors.New("invalid topic specifier, format: projects/PROJECTID/topics/TOPICNAME")
	}

	ctx := context.Background()

	client, err := pubsub.NewClient(ctx, parts[1])
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	topic := client.Topic(parts[3])

	var data []byte
	if m, ok := message.(StartMessage); ok {
		data, err = json.Marshal(m)
	} else {
		return errors.New("unknown message format")
	}

	if err != nil {
		return fmt.Errorf("marshalling message into json: %w", err)
	}
	logrus.Debugf("Message: %s", string(data))
	if _, err := topic.Publish(ctx, &pubsub.Message{Data: data}).Get(ctx); err != nil {
		return fmt.Errorf("publishing to pubsub topic: %w", err)
	}
	logrus.Infof("pushed build data to topic %s", topicString)
	return nil
}
