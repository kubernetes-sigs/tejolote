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

package run

import (
	"time"

	"github.com/puerco/tejolote/pkg/attestation"
)

type Run struct {
	SpecURL   string
	IsSuccess bool
	IsRunning bool
	Steps     []Step
	Artifacts []Artifact
	StartTime time.Time
	EndTime   time.Time
}

// Step is the interface that defines the behaviour of a build step
// the exec runner can execute
/*
type Step interface {
	Command() string
	Params() []string
}

*/
type Step struct {
	Command     string
	IsSuccess   bool
	Params      []string
	StartTime   time.Time
	EndTime     time.Time
	Environment map[string]string
}

// Artifact abstracts a file with the items we're interested in monitoring
type Artifact struct {
	Path     string
	Checksum map[string]string
	Time     time.Time
}

// Attest writes out the data of
func (r *Run) Attest() (*attestation.Attestation, error) {
	att := attestation.New().SLSA()
	return att, nil
}
