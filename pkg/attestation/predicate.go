/*
Copyright 2025 The Kubernetes Authors.

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

package attestation

import (
	"time"

	v1 "github.com/in-toto/attestation/go/v1"
)

type Predicate interface {
	SetBuilderID(string)
	SetBuilderType(string)
	SetInvocationID(string)
	SetConfigSource(*v1.ResourceDescriptor)
	SetEntryPoint(string)
	SetResolvedDependencies([]*v1.ResourceDescriptor)
	SetInternalParameters(map[string]any)
	AddExternalParameter(string, any)
	AddDependency(*v1.ResourceDescriptor)
	SetBuildConfig(map[string]any)
	SetStartedOn(*time.Time)
	SetFinishedOn(*time.Time)
	Type() string
}
