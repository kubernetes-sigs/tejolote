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

package snapshot

import "sigs.k8s.io/tejolote/pkg/run"

type Snapshot map[string]run.Artifact

// Delta takes a snapshot, assumed to be later in time and returns
// a directed delta, the files which were created or modified.
func (snap *Snapshot) Delta(post *Snapshot) []run.Artifact {
	results := []run.Artifact{}
	for path, f := range *post {
		// If the file was not there in the first snap, add it
		if _, ok := (*snap)[path]; !ok {
			results = append(results, f)
			continue
		}

		// Check the file attributes to if they were changed
		if (*snap)[path].Time != f.Time {
			results = append(results, f)
			continue
		}

		checksum := (*snap)[path].Checksum
		for algo, val := range checksum {
			if fv, ok := f.Checksum[algo]; ok {
				if fv != val {
					results = append(results, f)
					break
				}
			}
		}
	}
	return results
}
