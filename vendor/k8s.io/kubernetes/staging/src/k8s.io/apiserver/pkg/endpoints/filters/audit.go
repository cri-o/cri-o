/*
Copyright 2016 The Kubernetes Authors.

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

package filters

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pborman/uuid"

	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apiserver/pkg/endpoints/handlers/responsewriters"
	"k8s.io/apiserver/pkg/endpoints/request"
	authenticationapi "k8s.io/client-go/pkg/apis/authentication"
)

var _ http.ResponseWriter = &auditResponseWriter{}

type auditResponseWriter struct {
	http.ResponseWriter
	out io.Writer
	id  string
}

func (a *auditResponseWriter) WriteHeader(code int) {
	line := fmt.Sprintf("%s AUDIT: id=%q response=\"%d\"\n", time.Now().Format(time.RFC3339Nano), a.id, code)
	if _, err := fmt.Fprint(a.out, line); err != nil {
		glog.Errorf("Unable to write audit log: %s, the error is: %v", line, err)
	}

	a.ResponseWriter.WriteHeader(code)
}

// fancyResponseWriterDelegator implements http.CloseNotifier, http.Flusher and
// http.Hijacker which are needed to make certain http operation (e.g. watch, rsh, etc)
// working.
type fancyResponseWriterDelegator struct {
	*auditResponseWriter
}

func (f *fancyResponseWriterDelegator) CloseNotify() <-chan bool {
	return f.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (f *fancyResponseWriterDelegator) Flush() {
	f.ResponseWriter.(http.Flusher).Flush()
}

func (f *fancyResponseWriterDelegator) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return f.ResponseWriter.(http.Hijacker).Hijack()
}

var _ http.CloseNotifier = &fancyResponseWriterDelegator{}
var _ http.Flusher = &fancyResponseWriterDelegator{}
var _ http.Hijacker = &fancyResponseWriterDelegator{}

// WithAudit decorates a http.Handler with audit logging information for all the
// requests coming to the server. If out is nil, no decoration takes place.
// Each audit log contains two entries:
// 1. the request line containing:
//    - unique id allowing to match the response line (see 2)
//    - source ip of the request
//    - HTTP method being invoked
//    - original user invoking the operation
//    - impersonated user for the operation
//    - namespace of the request or <none>
//    - uri is the full URI as requested
// 2. the response line containing:
//    - the unique id from 1
//    - response code
func WithAudit(handler http.Handler, requestContextMapper request.RequestContextMapper, out io.Writer) http.Handler {
	if out == nil {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, ok := requestContextMapper.Get(req)
		if !ok {
			responsewriters.InternalError(w, req, errors.New("no context found for request"))
			return
		}
		attribs, err := GetAuthorizerAttributes(ctx)
		if err != nil {
			responsewriters.InternalError(w, req, err)
			return
		}

		username := "<none>"
		groups := "<none>"
		if attribs.GetUser() != nil {
			username = attribs.GetUser().GetName()
			if userGroups := attribs.GetUser().GetGroups(); len(userGroups) > 0 {
				groups = auditStringSlice(userGroups)
			}
		}
		asuser := req.Header.Get(authenticationapi.ImpersonateUserHeader)
		if len(asuser) == 0 {
			asuser = "<self>"
		}
		asgroups := "<lookup>"
		requestedGroups := req.Header[authenticationapi.ImpersonateGroupHeader]
		if len(requestedGroups) > 0 {
			asgroups = auditStringSlice(requestedGroups)
		}
		namespace := attribs.GetNamespace()
		if len(namespace) == 0 {
			namespace = "<none>"
		}
		id := uuid.NewRandom().String()

		line := fmt.Sprintf("%s AUDIT: id=%q ip=%q method=%q user=%q groups=%q as=%q asgroups=%q namespace=%q uri=%q\n",
			time.Now().Format(time.RFC3339Nano), id, utilnet.GetClientIP(req), req.Method, username, groups, asuser, asgroups, namespace, req.URL)
		if _, err := fmt.Fprint(out, line); err != nil {
			glog.Errorf("Unable to write audit log: %s, the error is: %v", line, err)
		}
		respWriter := decorateResponseWriter(w, out, id)
		handler.ServeHTTP(respWriter, req)
	})
}

func auditStringSlice(inList []string) string {
	if len(inList) == 0 {
		return ""
	}

	quotedElements := make([]string, len(inList))
	for i, in := range inList {
		quotedElements[i] = fmt.Sprintf("%q", in)
	}
	return strings.Join(quotedElements, ",")
}

func decorateResponseWriter(responseWriter http.ResponseWriter, out io.Writer, id string) http.ResponseWriter {
	delegate := &auditResponseWriter{ResponseWriter: responseWriter, out: out, id: id}
	// check if the ResponseWriter we're wrapping is the fancy one we need
	// or if the basic is sufficient
	_, cn := responseWriter.(http.CloseNotifier)
	_, fl := responseWriter.(http.Flusher)
	_, hj := responseWriter.(http.Hijacker)
	if cn && fl && hj {
		return &fancyResponseWriterDelegator{delegate}
	}
	return delegate
}
