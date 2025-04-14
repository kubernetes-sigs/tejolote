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

package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// TokenScopes returns the scopes of token in the eviroment
func TokenScopes() ([]string, error) {
	res, err := APIGetRequest("https://api.github.com/repos/github/docs")
	if err != nil {
		return nil, fmt.Errorf("making request to API: %w", err)
	}
	defer res.Body.Close()

	header := res.Header.Get("X-Oauth-Scopes")
	scopes := strings.Split(header, ", ")
	logrus.Debugf("GitHub Token scopes: %+v", scopes)
	return scopes, nil
}

// TokenHas returns a bool if the token in use has the scope passed
func TokenHas(scope string) (bool, error) {
	scopes, err := TokenScopes()
	if err != nil {
		return false, fmt.Errorf("reading scopes: %w", err)
	}
	for _, s := range scopes {
		if s == scope {
			return true, nil
		}
	}
	return false, nil
}

func APIGetRequest(url string) (*http.Response, error) {
	logrus.Debugf("GitHubAPI[GET]: %s", url)
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if os.Getenv("GITHUB_TOKEN") != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN")))
	} else {
		logrus.Warn("making unauthenticated request to github")
	}
	res, err := client.Do(req)
	if err != nil {
		return res, fmt.Errorf("executing http request to GitHub API: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"http error %d making request to GitHub API", res.StatusCode,
		)
	}
	return res, nil
}

func Download(url string, f io.Writer) error {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}

	if os.Getenv("GITHUB_TOKEN") != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", os.Getenv("GITHUB_TOKEN")))
	} else {
		logrus.Warn("making unauthenticated request to github")
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
	logrus.Infof("%d MB downloaded from %s", (numBytes / 1024 / 1024), url)
	return nil
}
