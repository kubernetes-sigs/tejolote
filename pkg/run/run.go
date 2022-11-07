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

package run

import (
	"time"
)

type Run struct {
	SpecURL    string
	IsSuccess  bool
	IsRunning  bool
	Params     []string
	Steps      []Step
	Artifacts  []Artifact
	StartTime  time.Time
	EndTime    time.Time
	SystemData interface{}
}

// Step is the interface that defines the behaviour of a build step
// the exec runner can execute
type Step struct {
	Command     string // Command run
	Image       string // Container image used for the step
	IsSuccess   bool
	Params      []string
	StartTime   time.Time // Start time of the step
	EndTime     time.Time
	Environment map[string]string
}

// Artifact abstracts a file with the items we're interested in monitoring
type Artifact struct {
	Path     string
	Checksum map[string]string
	Time     time.Time
}
