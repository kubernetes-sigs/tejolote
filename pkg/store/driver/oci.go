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
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type OCI struct {
	Repository string
	Image      string
}

func NewOCI(specURL string) (*OCI, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing SpecURL %s: %w", specURL, err)
	}
	if u.Path == "" {
		return nil, errors.New("spec url is not wel formed")
	}
	oci := &OCI{}
	parts := strings.Split(u.Path, "/")
	oci.Image = parts[len(parts)-1]
	oci.Repository = u.Hostname()
	if len(parts) > 1 {
		oci.Repository += strings.Join(parts[0:len(parts)-1], "/")
	}
	return oci, nil
}

// Snap
func (oci *OCI) Snap() (*snapshot.Snapshot, error) {
	tags, err := crane.ListTags(
		oci.Repository+"/"+oci.Image, crane.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		return nil, fmt.Errorf("fetching tags from registry: %w", err)
	}
	snap := &snapshot.Snapshot{}
	for _, t := range tags {
		(*snap)["oci://"+t] = run.Artifact{
			Path:     "oci://" + oci.Repository + "/" + oci.Image + ":" + t,
			Checksum: map[string]string{},
			Time:     time.Time{},
		}
	}
	return snap, nil
}
