// Copyright 2021 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ctl

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sigstore/fulcio/pkg/ca"
)

const addChainPath = "ct/v1/add-chain"

type Client struct {
	c   *http.Client
	url string
}

func New(url string) *Client {
	c := &http.Client{Timeout: 30 * time.Second}
	return &Client{
		c:   c,
		url: url,
	}
}

type certChain struct {
	Chain []string `json:"chain"`
}

type CertChainResponse struct {
	SctVersion int    `json:"sct_version"`
	ID         string `json:"id"`
	Timestamp  int64  `json:"timestamp"`
	Extensions string `json:"extensions"`
	Signature  string `json:"signature"`
}

type ErrorResponse struct {
	StatusCode int    `json:"statusCode"`
	ErrorCode  string `json:"errorCode"`
	Message    string `json:"message"`
}

func (err *ErrorResponse) Error() string {
	if err.ErrorCode == "" {
		return fmt.Sprintf("%d CT API error: %s", err.StatusCode, err.Message)
	}
	return fmt.Sprintf("%d (%s) CT API error: %s", err.StatusCode, err.ErrorCode, err.Message)
}

func (c *Client) AddChain(csc *ca.CodeSigningCertificate) (*CertChainResponse, error) {
	chainjson := &certChain{Chain: []string{
		base64.StdEncoding.EncodeToString(csc.FinalCertificate.Raw),
	}}

	for _, c := range csc.FinalChain {
		chainjson.Chain = append(chainjson.Chain, base64.StdEncoding.EncodeToString(c.Raw))
	}
	jsonStr, err := json.Marshal(chainjson)
	if err != nil {
		return nil, err
	}

	// Send to add-chain on CT log
	url := fmt.Sprintf("%s/%s", c.url, addChainPath)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.c.Do(req)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case 200:
		var ctlResp CertChainResponse
		if err := json.NewDecoder(resp.Body).Decode(&ctlResp); err != nil {
			return nil, err
		}
		return &ctlResp, nil
	case 400, 401, 403, 500:
		var errRes ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errRes); err != nil {
			return nil, err
		}

		if errRes.StatusCode == 0 {
			errRes.StatusCode = resp.StatusCode
		}
		return nil, &errRes
	default:
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
}
