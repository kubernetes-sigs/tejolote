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

package git

import (
	"errors"
	"fmt"
	"path/filepath"

	gogit "github.com/go-git/go-git/v5"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-utils/util"
)

const defaultRemote = "origin"

type Repository struct {
	Options Options
}

func NewRepo(dir string) *Repository {
	return &Repository{
		Options: Options{
			CWD: dir,
		},
	}
}

type Options struct {
	CWD string
}

// SourceURL returns the repository URL
func (r *Repository) SourceURL() (string, error) {
	if !util.Exists(filepath.Join(r.Options.CWD, "/.git")) {
		logrus.Debugf("Directory %s is not a git repository", r.Options.CWD)
		return "", nil
	}

	repo, err := gogit.PlainOpen(r.Options.CWD)
	if err != nil {
		return "", fmt.Errorf("opening git repo at %s: %w", r.Options.CWD, err)
	}

	remote, err := repo.Remote(defaultRemote)
	if err != nil {
		return "", fmt.Errorf("getting repository remote: %w", err)
	}

	if len(remote.Config().URLs) == 0 {
		return "", errors.New("repo remote does not have URLs")
	}

	return remote.Config().URLs[0], nil
}
