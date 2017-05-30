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
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit/policy"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	pluginlog "k8s.io/apiserver/plugin/pkg/audit/log"
)

type fakeAuditSink struct {
	lock   sync.Mutex
	events []*auditinternal.Event
}

func (s *fakeAuditSink) ProcessEvents(evs ...*auditinternal.Event) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.events = append(s.events, evs...)
}

func (s *fakeAuditSink) Events() []*auditinternal.Event {
	s.lock.Lock()
	defer s.lock.Unlock()
	return append([]*auditinternal.Event{}, s.events...)
}

func (s *fakeAuditSink) Pop(timeout time.Duration) (*auditinternal.Event, error) {
	var result *auditinternal.Event
	err := wait.Poll(50*time.Millisecond, wait.ForeverTestTimeout, wait.ConditionFunc(func() (done bool, err error) {
		s.lock.Lock()
		defer s.lock.Unlock()
		if len(s.events) == 0 {
			return false, nil
		}
		result = s.events[0]
		s.events = s.events[1:]
		return true, nil
	}))
	return result, err
}

type simpleResponseWriter struct{}

var _ http.ResponseWriter = &simpleResponseWriter{}

func (*simpleResponseWriter) WriteHeader(code int)         {}
func (*simpleResponseWriter) Write(bs []byte) (int, error) { return len(bs), nil }
func (*simpleResponseWriter) Header() http.Header          { return http.Header{} }

type fancyResponseWriter struct {
	simpleResponseWriter
}

func (*fancyResponseWriter) CloseNotify() <-chan bool { return nil }

func (*fancyResponseWriter) Flush() {}

func (*fancyResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) { return nil, nil, nil }

func TestConstructResponseWriter(t *testing.T) {
	actual := decorateResponseWriter(&simpleResponseWriter{}, nil, nil)
	switch v := actual.(type) {
	case *auditResponseWriter:
	default:
		t.Errorf("Expected auditResponseWriter, got %v", reflect.TypeOf(v))
	}

	actual = decorateResponseWriter(&fancyResponseWriter{}, nil, nil)
	switch v := actual.(type) {
	case *fancyResponseWriterDelegator:
	default:
		t.Errorf("Expected fancyResponseWriterDelegator, got %v", reflect.TypeOf(v))
	}
}

func TestDecorateResponseWriterWithoutChannel(t *testing.T) {
	ev := &auditinternal.Event{}
	actual := decorateResponseWriter(&simpleResponseWriter{}, ev, nil)

	// write status. This will not block because firstEventSentCh is nil
	actual.WriteHeader(42)
	if ev.ResponseStatus == nil {
		t.Fatalf("Expected ResponseStatus to be non-nil")
	}
	if ev.ResponseStatus.Code != 42 {
		t.Errorf("expected status code 42, got %d", ev.ResponseStatus.Code)
	}
}

func TestDecorateResponseWriterWithImplicitWrite(t *testing.T) {
	ev := &auditinternal.Event{}
	actual := decorateResponseWriter(&simpleResponseWriter{}, ev, nil)

	// write status. This will not block because firstEventSentCh is nil
	actual.Write([]byte("foo"))
	if ev.ResponseStatus == nil {
		t.Fatalf("Expected ResponseStatus to be non-nil")
	}
	if ev.ResponseStatus.Code != 200 {
		t.Errorf("expected status code 200, got %d", ev.ResponseStatus.Code)
	}
}

func TestDecorateResponseWriterChannel(t *testing.T) {
	sink := &fakeAuditSink{}
	ev := &auditinternal.Event{}
	actual := decorateResponseWriter(&simpleResponseWriter{}, ev, sink)

	done := make(chan struct{})
	go func() {
		t.Log("Writing status code 42")
		actual.WriteHeader(42)
		t.Log("Finished writing status code 42")
		close(done)

		actual.Write([]byte("foo"))
	}()

	// sleep some time to give write the possibility to do wrong stuff
	time.Sleep(100 * time.Millisecond)

	t.Log("Waiting for event in the channel")
	ev1, err := sink.Pop(time.Second)
	if err != nil {
		t.Fatal("Timeout waiting for events")
	}
	t.Logf("Seen event with status %v", ev1.ResponseStatus)

	if ev != ev1 {
		t.Fatalf("ev1 and ev must be equal")
	}

	<-done
	t.Log("Seen the go routine finished")

	// write again
	_, err = actual.Write([]byte("foo"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

type fakeHTTPHandler struct{}

func (*fakeHTTPHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(200)
}

func TestAudit(t *testing.T) {
	shortRunningPrefix := `[\d\:\-\.\+TZ]+ AUDIT: id="[\w-]+" ip="127.0.0.1" method="list" user="admin" groups="<none>" as="<self>" asgroups="<lookup>" namespace="default" uri="/api/v1/namespaces/default/pods"`
	longRunningPrefix := `[\d\:\-\.\+TZ]+ AUDIT: id="[\w-]+" ip="127.0.0.1" method="watch" user="admin" groups="<none>" as="<self>" asgroups="<lookup>" namespace="default" uri="/api/v1/namespaces/default/pods\?watch=true"`

	shortRunningPath := "/api/v1/namespaces/default/pods"
	longRunningPath := "/api/v1/namespaces/default/pods?watch=true"

	delay := 500 * time.Millisecond

	for _, test := range []struct {
		desc     string
		path     string
		handler  func(http.ResponseWriter, *http.Request)
		expected []string
	}{
		// short running requests
		{
			"empty",
			shortRunningPath,
			func(http.ResponseWriter, *http.Request) {},
			[]string{
				shortRunningPrefix + ` response="200"`,
			},
		},
		{
			"sleep",
			shortRunningPath,
			func(http.ResponseWriter, *http.Request) {
				time.Sleep(delay)
			},
			[]string{
				shortRunningPrefix + ` response="200"`,
			},
		},
		{
			"403+write",
			shortRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(403)
				w.Write([]byte("foo"))
			},
			[]string{
				shortRunningPrefix + ` response="403"`,
			},
		},
		{
			"panic",
			shortRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				panic("kaboom")
			},
			[]string{
				shortRunningPrefix + ` response="500"`,
			},
		},
		{
			"write+panic",
			shortRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("foo"))
				panic("kaboom")
			},
			[]string{
				shortRunningPrefix + ` response="500"`,
			},
		},

		// long running requests
		{
			"empty longrunning",
			longRunningPath,
			func(http.ResponseWriter, *http.Request) {},
			[]string{
				longRunningPrefix + ` response="200"`,
			},
		},
		{
			"sleep longrunning",
			longRunningPath,
			func(http.ResponseWriter, *http.Request) {
				time.Sleep(delay)
			},
			[]string{
				longRunningPrefix + ` response="200"`,
			},
		},
		{
			"sleep+403 longrunning",
			longRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(delay)
				w.WriteHeader(403)
			},
			[]string{
				longRunningPrefix + ` response="<deferred>"`,
				longRunningPrefix + ` response="403"`,
			},
		},
		{
			"write longrunning",
			longRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("foo"))
			},
			[]string{
				longRunningPrefix + ` response="<deferred>"`,
				longRunningPrefix + ` response="200"`,
			},
		},
		{
			"403+write longrunning",
			longRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(403)
				w.Write([]byte("foo"))
			},
			[]string{
				longRunningPrefix + ` response="<deferred>"`,
				longRunningPrefix + ` response="403"`,
			},
		},
		{
			"panic longrunning",
			longRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				panic("kaboom")
			},
			[]string{
				longRunningPrefix + ` response="500"`,
			},
		},
		{
			"write+panic longrunning",
			longRunningPath,
			func(w http.ResponseWriter, req *http.Request) {
				w.Write([]byte("foo"))
				panic("kaboom")
			},
			[]string{
				longRunningPrefix + ` response="<deferred>"`,
				longRunningPrefix + ` response="500"`,
			},
		},
	} {
		var buf bytes.Buffer
		backend := pluginlog.NewBackend(&buf)
		policyChecker := policy.FakeChecker(auditinternal.LevelRequestResponse)
		handler := WithAudit(http.HandlerFunc(test.handler), &fakeRequestContextMapper{
			user: &user.DefaultInfo{Name: "admin"},
		}, backend, policyChecker, func(r *http.Request, ri *request.RequestInfo) bool {
			// simplified long-running check
			return ri.Verb == "watch"
		})

		req, _ := http.NewRequest("GET", test.path, nil)
		req.RemoteAddr = "127.0.0.1"

		func() {
			defer func() {
				recover()
			}()
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()

		t.Logf("[%s] audit log: %v", test.desc, buf.String())

		line := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(line) != len(test.expected) {
			t.Errorf("[%s] Unexpected amount of lines in audit log: %d", test.desc, len(line))
			continue
		}

		for i, re := range test.expected {
			match, err := regexp.MatchString(re, line[i])
			if err != nil {
				t.Errorf("[%s] Unexpected error matching line %d: %v", test.desc, i, err)
				continue
			}
			if !match {
				t.Errorf("[%s] Unexpected line %d of audit: %s", test.desc, i, line[i])
			}
		}
	}
}

type fakeRequestContextMapper struct {
	user *user.DefaultInfo
}

func (m *fakeRequestContextMapper) Get(req *http.Request) (request.Context, bool) {
	ctx := request.NewContext()
	if m.user != nil {
		ctx = request.WithUser(ctx, m.user)
	}

	resolver := newTestRequestInfoResolver()
	info, err := resolver.NewRequestInfo(req)
	if err == nil {
		ctx = request.WithRequestInfo(ctx, info)
	}

	return ctx, true
}

func (*fakeRequestContextMapper) Update(req *http.Request, context request.Context) error {
	return nil
}

func TestAuditNoPanicOnNilUser(t *testing.T) {
	policyChecker := policy.FakeChecker(auditinternal.LevelRequestResponse)
	handler := WithAudit(&fakeHTTPHandler{}, &fakeRequestContextMapper{}, &fakeAuditSink{}, policyChecker, nil)
	req, _ := http.NewRequest("GET", "/api/v1/namespaces/default/pods", nil)
	req.RemoteAddr = "127.0.0.1"
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestAuditLevelNone(t *testing.T) {
	sink := &fakeAuditSink{}
	var handler http.Handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	policyChecker := policy.FakeChecker(auditinternal.LevelNone)
	handler = WithAudit(handler, &fakeRequestContextMapper{
		user: &user.DefaultInfo{Name: "admin"},
	}, sink, policyChecker, nil)

	req, _ := http.NewRequest("GET", "/api/v1/namespaces/default/pods", nil)
	req.RemoteAddr = "127.0.0.1"

	handler.ServeHTTP(httptest.NewRecorder(), req)
	if len(sink.events) > 0 {
		t.Errorf("Generated events, but should not have: %#v", sink.events)
	}
}
