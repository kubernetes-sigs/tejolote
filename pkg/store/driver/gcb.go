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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/cloudbuild/v1"

	"sigs.k8s.io/release-utils/hash"

	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type GCB struct {
	ProjectID string
	BuildID   string
	client    *storage.Client
}

func NewGCB(specURL string) (*GCB, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing GCB spec URL: %w", err)
	}

	ctx := context.Background()
	client, err := newGCSClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating storage client: %w", err)
	}

	return &GCB{
		ProjectID: u.Hostname(),
		BuildID:   strings.TrimPrefix(u.Path, "/"),
		client:    client,
	}, nil
}

func (gcb *GCB) readArtifacts() ([]run.Artifact, error) {
	ctx := context.Background()
	cloudbuildService, err := cloudbuild.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating cloudbuild client: %w", err)
	}
	build, err := cloudbuildService.Projects.Builds.Get(gcb.ProjectID, gcb.BuildID).Do()
	if err != nil {
		return nil, fmt.Errorf("getting build %s from GCB: %w", gcb.BuildID, err)
	}
	manifest := build.Results.ArtifactManifest
	if manifest == "" {
		logrus.Info("no artifact manifest in run, assuming no artifacts")
		return []run.Artifact{}, nil
	}

	logrus.Infof("pulling artifact manifest from %s", manifest)

	// Get the artifacts list from th build service
	gcbArtifacts, err := gcb.readArtifactManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("reading build artifact manifest: %w", err)
	}
	logrus.Debugf("%+v", gcbArtifacts)

	// Hash the artifacts list
	var wg errgroup.Group
	var mtx sync.Mutex
	artifacts := []run.Artifact{}
	for _, artifactData := range gcbArtifacts {
		artifactData := artifactData
		wg.Go(func() error {
			f, err := os.CreateTemp("", "artifact-temp-")
			if err != nil {
				return fmt.Errorf("creating temporary artifact file")
			}
			defer os.Remove(f.Name())

			if err := downloadGCSObject(gcb.client, artifactData.Location, f); err != nil {
				return fmt.Errorf("downloading artifact: %w", err)
			}

			attrs, err := readGCSObjectAttributes(gcb.client, artifactData.Location)
			if err != nil {
				return fmt.Errorf("reading object artifacts: %w", err)
			}

			hashValue, err := hash.SHA256ForFile(f.Name())
			if err != nil {
				return fmt.Errorf("hashing artifact: %w", err)
			}
			mtx.Lock()
			artifacts = append(artifacts, run.Artifact{
				Path: artifactData.Location,
				Checksum: map[string]string{
					"SHA256": hashValue,
				},
				Time: attrs.Updated,
			})
			mtx.Unlock()
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, fmt.Errorf("hashing artifacts: %w", err)
	}
	return artifacts, nil
}

func parseGCSObjectURL(objectURL string) (bucket, path string, err error) {
	u, err := url.Parse(objectURL)
	if err != nil {
		return bucket, path, fmt.Errorf("parsing GCS object URL: %w", err)
	}
	if u.Scheme != "gs" {
		return bucket, path, errors.New("url is not a cloud storage URL")
	}
	return u.Hostname(), u.Path, nil
}

// Abstract object data as found in the GCB artifact manifest
type ghcsManifestArtifact struct {
	Location string `json:"location"`
	FileHash []struct {
		FileHash []struct {
			Type  int    `json:"type"`
			Value string `json:"value"`
		} `json:"file_hash"`
	} `json:"file_hash"`
}

func readGCSObjectAttributes(client *storage.Client, objectURL string) (*storage.ObjectAttrs, error) {
	bucket, path, err := parseGCSObjectURL(objectURL)
	if err != nil {
		return nil, fmt.Errorf("parsing GCS url: %w", err)
	}

	// Create the reader to copy data
	attrs, err := client.Bucket(bucket).Object(strings.TrimPrefix(path, "/")).Attrs(context.Background())
	if err != nil {
		return nil, fmt.Errorf("creating bucket reader: %w", err)
	}
	logrus.Debugf("%+v", attrs)
	return attrs, nil
}

func downloadGCSObject(client *storage.Client, objectURL string, f io.Writer) error {
	bucket, path, err := parseGCSObjectURL(objectURL)
	if err != nil {
		return fmt.Errorf("parsing GCS url: %w", err)
	}

	// Create the reader to copy data
	rc, err := client.Bucket(bucket).Object(strings.TrimPrefix(path, "/")).NewReader(context.Background())
	if err != nil {
		return fmt.Errorf("creating bucket reader: %w", err)
	}
	defer rc.Close()
	var b int64
	if b, err = io.Copy(f, rc); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}
	logrus.Debugf("Wrote %d bytes from %s", b, objectURL)
	return nil
}

// Downloads the manifest from the bucket
func (gcb *GCB) readArtifactManifest(manifestURL string) ([]ghcsManifestArtifact, error) {
	var b bytes.Buffer

	if err := downloadGCSObject(gcb.client, manifestURL, &b); err != nil {
		return nil, fmt.Errorf("reading manifest from GCS: %w", err)
	}

	dec := json.NewDecoder(strings.NewReader(b.String()))
	ret := []ghcsManifestArtifact{}
	logrus.Infof("JSON: %s", b.String())
	for {
		var a ghcsManifestArtifact
		if err := dec.Decode(&a); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s\n", a.Location)
		ret = append(ret, a)
	}
	return ret, nil
}

func (gcb *GCB) Snap() (*snapshot.Snapshot, error) {
	snap := snapshot.Snapshot{}
	artifacts, err := gcb.readArtifacts()
	if err != nil {
		return nil, fmt.Errorf("reading artifacts: %w", err)
	}

	for _, a := range artifacts {
		snap[a.Path] = a
	}

	return &snap, err
}
