/*
Copyright 2020 The Kubernetes Authors.

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

package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

// GetURLResponse returns the HTTP response for the provided URL if the request succeeds
func GetURLResponse(url string, trim bool) (string, error) {
	resp, httpErr := http.Get(url)
	if httpErr != nil {
		return "", errors.Wrapf(httpErr, "an error occurred GET-ing %s", url)
	}

	defer resp.Body.Close()
	statusOK := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !statusOK {
		errMsg := fmt.Sprintf("HTTP status not OK (%v) for %s", resp.StatusCode, url)
		return "", errors.New(errMsg)
	}

	respBytes, ioErr := ioutil.ReadAll(resp.Body)
	if ioErr != nil {
		return "", errors.Wrapf(ioErr, "could not handle the response body for %s", url)
	}

	respString := string(respBytes)
	if trim {
		respString = strings.TrimSpace(respString)
	}

	return respString, nil
}
