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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	intoto "github.com/in-toto/in-toto-golang/in_toto"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/tejolote/pkg/run"
	"sigs.k8s.io/tejolote/pkg/store/snapshot"
)

type Attestation struct {
	URL string
}

func NewAttestation(specURL string) (*Attestation, error) {
	u, err := url.Parse(specURL)
	if err != nil {
		return nil, fmt.Errorf("parsing attestation spec url: %w", err)
	}
	if !strings.HasPrefix(u.Scheme, "intoto+") {
		return nil, fmt.Errorf("spec URL %s is not an attestation url", u.Scheme)
	}
	logrus.Infof(
		"Initialized new in-toto attestation storage backend (%s)", specURL,
	)
	// TODO: Check scheme to make sure it is valid
	return &Attestation{
		URL: strings.TrimPrefix(specURL, "intoto+"),
	}, nil
}

// downloadURL universal download function
// TODO: Move these to methods in each driver
func downloadURL(sourceURL string, w io.Writer) error {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("parsing url %w", err)
	}
	ctx := context.Background()
	switch u.Scheme {
	case "gs":
		client, err := newGCSClient(ctx)
		if err != nil {
			return fmt.Errorf("creating GCS client: %w", err)
		}
		return downloadGCSObject(client, sourceURL, w)
	case "http", "https":
		return downloadHTTP(sourceURL, w)
	case "file":
		f, err := os.Open(strings.TrimPrefix(sourceURL, "file://"))
		if err != nil {
			return fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()
		if _, err := io.Copy(w, f); err != nil {
			return fmt.Errorf("reading file data: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("scheme not supported for single downloads")
	}
}

func (att *Attestation) Snap() (*snapshot.Snapshot, error) {
	inTotoAtt := intoto.Statement{}
	// Parse the attestation
	rawData, err := att.downloadAttestation()
	if err != nil {
		return nil, fmt.Errorf("downloading attestation data: %w", err)
	}

	// Parse the json data
	if err := json.Unmarshal(rawData, &inTotoAtt); err != nil {
		return nil, fmt.Errorf("unmarshalling attestation data: %w", err)
	}
	snap := snapshot.Snapshot{}
	if inTotoAtt.Subject == nil {
		return &snap, nil
	}

	for _, s := range inTotoAtt.Subject {
		snap[s.Name] = run.Artifact{
			Path:     s.Name,
			Checksum: map[string]string{},
		}
		for h, val := range s.Digest {
			snap[s.Name].Checksum[h] = val
		}
	}
	return &snap, nil
}

func (att *Attestation) downloadAttestation() ([]byte, error) {
	var b bytes.Buffer
	if err := downloadURL(att.URL, &b); err != nil {
		return nil, fmt.Errorf("downloading attestation data: %w", err)
	}
	return b.Bytes(), nil
}

func downloadHTTP(urlPath string, f io.Writer) error {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, urlPath, nil)
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing http request to GitHub API: %w", err)
	}

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http error when downloading: %s", resp.Status)
	}

	defer resp.Body.Close()

	// Writer the body to file
	numBytes, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("writing http response to disk: %w", err)
	}
	logrus.Debugf("%d MB downloaded from %s", (numBytes / 1024 / 1024), urlPath)
	return nil
}
