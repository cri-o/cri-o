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

package api

import (
	"fmt"
	"net/http"

	"github.com/sigstore/fulcio/pkg/log"
)

const (
	invalidSignature          = "The signature supplied in the request could not be verified"
	invalidCertificateRequest = "The CertificateRequest was invalid"
	invalidPublicKey          = "The public key supplied in the request could not be parsed"
	failedToEnterCertInCTL    = "Error entering certificate in CTL @ '%v'"
	failedToMarshalSCT        = "Error marshaling signed certificate timestamp"
	failedToMarshalCert       = "Error marshaling code signing certificate"
	//nolint
	invalidCredentials = "There was an error processing the credentials for this request"
	genericCAError     = "error communicating with CA backend"
)

func handleFulcioAPIError(w http.ResponseWriter, req *http.Request, code int, err error, message string, fields ...interface{}) {
	if message == "" {
		message = http.StatusText(code)
	}

	log.RequestIDLogger(req).Errorw("exiting with error", append([]interface{}{"handler", req.URL.Path, "statusCode", code, "clientMessage", message, "error", err}, fields...)...)
	http.Error(w, fmt.Sprintf(`{"code":%d,"message":%q}`, code, message), code)
}
