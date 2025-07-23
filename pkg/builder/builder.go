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

package builder

import (
	"fmt"
	"strings"

	v1 "github.com/in-toto/attestation/go/v1"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/tejolote/pkg/attestation"
	"sigs.k8s.io/tejolote/pkg/builder/driver"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store"
)

type Builder struct {
	SpecURL string
	VCSURL  string
	driver  driver.BuildSystem
}

// New returns a new builder loaded with the driver derived from
// the spec URL
func New(spec string) (bldr Builder, err error) {
	bldr = Builder{
		SpecURL: spec,
	}

	d, err := driver.NewFromSpecURL(spec)
	if err != nil {
		return bldr, fmt.Errorf("getting driver: %w", err)
	}

	bldr.driver = d
	return bldr, nil
}

func (b *Builder) Snap() error {
	return nil
}

func (b *Builder) GetRun(identifier string) (*run.Run, error) {
	return b.driver.GetRun(identifier)
}

// RefreshRun refreshes a run with the latest data from
// the build system
func (b *Builder) RefreshRun(r *run.Run) error {
	return b.driver.RefreshRun(r)
}

func (b *Builder) BuildPredicate(r *run.Run, draft attestation.Predicate) (attestation.Predicate, error) {
	pred, err := b.driver.BuildPredicate(r, draft)
	if err != nil {
		return nil, err
	}
	// If there is a VCS URL set, add it to the predicate
	if b.VCSURL != "" {
		u, commit, ok := strings.Cut(b.VCSURL, "@")
		des := &v1.ResourceDescriptor{
			Uri: u,
		}
		if ok {
			// The thing after the @ may not be a commit
			if len(commit) == 40 {
				des.Digest["sha1"] = commit
				des.Digest["gitCommit"] = commit
			} else {
				des.Uri = b.VCSURL
			}
			pred.AddDependency(des)
		} else {
			logrus.Warn("unable to read commit from vcs url")
			pred.AddDependency(des)
		}
	}

	if r.BuildPoint != nil {
		pred.AddDependency(
			&v1.ResourceDescriptor{
				Uri:    r.BuildPoint.GetUri(),
				Digest: r.BuildPoint.GetDigest(),
			},
		)
	}
	return pred, nil
}

func (b *Builder) ArtifactStores() []store.Store {
	return b.driver.ArtifactStores()
}
