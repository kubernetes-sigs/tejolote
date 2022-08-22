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

package snapshot

import (
	"testing"
	"time"

	"github.com/puerco/tejolote/pkg/run"
	"github.com/stretchr/testify/require"
)

func TestDelta(t *testing.T) {
	testFile := run.Artifact{
		Path:     "test.txt",
		Checksum: map[string]string{"SHA256": "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4"},
		Time:     time.Now(),
	}
	modHashFile := run.Artifact{
		Path:     "test.txt",
		Checksum: map[string]string{"SHA256": "25b89320221dda5abe3df4624d246d22d0c820ee3598e97553611d7c80abbd36"},
		Time:     testFile.Time,
	}
	modTimeFile := run.Artifact{
		Path:     "test.txt",
		Checksum: map[string]string{"SHA256": "25b89320221dda5abe3df4624d246d22d0c820ee3598e97553611d7c80abbd36"},
		Time:     time.Date(1976, time.Month(2), 10, 23, 30, 30, 0, time.Local),
	}
	for _, tc := range []struct {
		preSnap  Snapshot
		postSnap Snapshot
		expect   []run.Artifact
	}{
		{
			// Empty snapshots, should be an empty list
			Snapshot{},
			Snapshot{},
			[]run.Artifact{},
		},
		{
			// One removed file, should be empty list
			Snapshot{testFile.Path: testFile},
			Snapshot{},
			[]run.Artifact{},
		},
		{
			// One added file, should be a list with that file
			Snapshot{},
			Snapshot{testFile.Path: testFile},
			[]run.Artifact{testFile},
		},
		{
			// One file with time modded, should be a list with that file
			Snapshot{testFile.Path: testFile},
			Snapshot{testFile.Path: modTimeFile},
			[]run.Artifact{modTimeFile},
		},
		{
			// One file with hash modded, should be a list with that file
			Snapshot{testFile.Path: testFile},
			Snapshot{testFile.Path: modHashFile},
			[]run.Artifact{modHashFile},
		},
	} {
		require.Equal(t, tc.expect, tc.preSnap.Delta(&tc.postSnap))
	}
}
