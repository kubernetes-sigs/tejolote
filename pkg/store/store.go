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

package store

import (
	"fmt"
	"net/url"

	"github.com/puerco/tejolote/pkg/run"
	"github.com/puerco/tejolote/pkg/store/driver"
	"github.com/puerco/tejolote/pkg/store/snapshot"
)

type Store struct {
	SpecURL string
	Driver  Implementation
	Snaps   []snapshot.Snapshot
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
	case "file://":
		impl, err = driver.NewDirectory(specURL)
		if err != nil {
			return s, fmt.Errorf("generating new directory: %w", err)
		}
	default:
		return s, fmt.Errorf("%s is not a storage URL", specURL)
	}
	s.Driver = impl

	return s, nil
}

func (s *Store) ReadArtifacts() ([]run.Artifact, error) {
	return nil, nil
}
