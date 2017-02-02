// Copyright 2016 The Linux Foundation
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

package schema

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

type _escLocalFS struct{}

var _escLocal _escLocalFS

type _escStaticFS struct{}

var _escStatic _escStaticFS

type _escDirectory struct {
	fs   http.FileSystem
	name string
}

type _escFile struct {
	compressed string
	size       int64
	modtime    int64
	local      string
	isDir      bool

	once sync.Once
	data []byte
	name string
}

func (_escLocalFS) Open(name string) (http.File, error) {
	f, present := _escData[path.Clean(name)]
	if !present {
		return nil, os.ErrNotExist
	}
	return os.Open(f.local)
}

func (_escStaticFS) prepare(name string) (*_escFile, error) {
	f, present := _escData[path.Clean(name)]
	if !present {
		return nil, os.ErrNotExist
	}
	var err error
	f.once.Do(func() {
		f.name = path.Base(name)
		if f.size == 0 {
			return
		}
		var gr *gzip.Reader
		b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewBufferString(f.compressed))
		gr, err = gzip.NewReader(b64)
		if err != nil {
			return
		}
		f.data, err = ioutil.ReadAll(gr)
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (fs _escStaticFS) Open(name string) (http.File, error) {
	f, err := fs.prepare(name)
	if err != nil {
		return nil, err
	}
	return f.File()
}

func (dir _escDirectory) Open(name string) (http.File, error) {
	return dir.fs.Open(dir.name + name)
}

func (f *_escFile) File() (http.File, error) {
	type httpFile struct {
		*bytes.Reader
		*_escFile
	}
	return &httpFile{
		Reader:   bytes.NewReader(f.data),
		_escFile: f,
	}, nil
}

func (f *_escFile) Close() error {
	return nil
}

func (f *_escFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func (f *_escFile) Stat() (os.FileInfo, error) {
	return f, nil
}

func (f *_escFile) Name() string {
	return f.name
}

func (f *_escFile) Size() int64 {
	return f.size
}

func (f *_escFile) Mode() os.FileMode {
	return 0
}

func (f *_escFile) ModTime() time.Time {
	return time.Unix(f.modtime, 0)
}

func (f *_escFile) IsDir() bool {
	return f.isDir
}

func (f *_escFile) Sys() interface{} {
	return f
}

// _escFS returns a http.Filesystem for the embedded assets. If useLocal is true,
// the filesystem's contents are instead used.
func _escFS(useLocal bool) http.FileSystem {
	if useLocal {
		return _escLocal
	}
	return _escStatic
}

// _escDir returns a http.Filesystem for the embedded assets on a given prefix dir.
// If useLocal is true, the filesystem's contents are instead used.
func _escDir(useLocal bool, name string) http.FileSystem {
	if useLocal {
		return _escDirectory{fs: _escLocal, name: name}
	}
	return _escDirectory{fs: _escStatic, name: name}
}

// _escFSByte returns the named file from the embedded assets. If useLocal is
// true, the filesystem's contents are instead used.
func _escFSByte(useLocal bool, name string) ([]byte, error) {
	if useLocal {
		f, err := _escLocal.Open(name)
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(f)
		f.Close()
		return b, err
	}
	f, err := _escStatic.prepare(name)
	if err != nil {
		return nil, err
	}
	return f.data, nil
}

// _escFSMustByte is the same as _escFSByte, but panics if name is not present.
func _escFSMustByte(useLocal bool, name string) []byte {
	b, err := _escFSByte(useLocal, name)
	if err != nil {
		panic(err)
	}
	return b
}

// _escFSString is the string version of _escFSByte.
func _escFSString(useLocal bool, name string) (string, error) {
	b, err := _escFSByte(useLocal, name)
	return string(b), err
}

// _escFSMustString is the string version of _escFSMustByte.
func _escFSMustString(useLocal bool, name string) string {
	return string(_escFSMustByte(useLocal, name))
}

var _escData = map[string]*_escFile{

	"/config-schema.json": {
		local:   "config-schema.json",
		size:    774,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/5SRvW7rMAyFdz+F4WS8ju7QKWsfoEPHooMqUzEDWFRJZgiKvHv1EzcxEBTuEsSH/M4R
ya+mbbsBxDFGRQrdvu1eIoRnCmoxALfpn8dD+xrBoUdnS9e/jG3FjTDZjIyqcW/MUSj0Vd0RH8zA1mv/
/8lUbVM5HGZEEkMpzc1pUrDabXCyBzCu5FdSzxEySx9HcFq1yMmBFUFSJY+TNMdgFYYf4Q4VZQzVruie
eLKaK0NCesUJulK71JbOnnQk/sVq2c1uRE2POzGsZUjWdl53cde9ZfDl8eClr+VdvsLGJAUD5mvJvMOF
FxOpl797XbmF14iixOdHY1hme76tO+1mug9dHTtHXLlLM/+WN3QMnyfkcvK3B5e4bXo5ffp4by7NdwAA
AP//XlvgsQYDAAA=
`,
	},

	"/content-descriptor.json": {
		local:   "content-descriptor.json",
		size:    956,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/5STP2/bMBDFd3+KgxIgSxx1CDoYQZZ279BuRQeaPEmXmn96PCNVC3/3HkUpttsigRfD
fHyP9zvy9HsF0DjMlikJxdBsoPmUMHyIQQwFZCj/MAh8nE2R4XNCSx1ZMyVuyxHX2Q7oTYkPImnTtk85
hnVV7yL3rWPTyfrdfVu1q5ojt0SyZqJWtkvlPMWqu3Uv1WtOxoQlGbdPaKVqiTXPQph1pzSmmkdH5ks1
V+nffmVAmHzlUIgdFIGxQ1YadHBSY4pf617JOezymrzp8a40e6WQHQUqx+b2WHiKHWq6yfTrLZRiAQqw
HQXzhTj/AaEg7+/PIRz1mOUNDMujXnfPJg1kQV/Bfs97DzW7YFWW24JblsmIIAe4eRhMHh43DwP+NE6H
xZvdnHy8ufAiZ9izBva8y6/gG9hRZSxG6Dh6eNYuBoWkPEODNyNsEVx8DruolO4ItkyXYTbjUSZBf1r3
xJmFKfQvVt3pIntTLllpqZn1w2r5nVppGH/sibF8BF//HtjTiTl/OF18Wx1WfwIAAP//z1UgVLwDAAA=
`,
	},

	"/defs-config.json": {
		local:   "defs-config.json",
		size:    2236,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/+RVzY7TMBC+5yksw7Gwd6673JCKVAEHhCo3Ge/OEnvMeIKI0L47TrZ0888qpac9VG0m
/n7m89j9nSmlC4g5YxAkr98pfQMWPTZPUQXDgnlVGlZCahvAX5MXgx5YpV8Wb9UuQI4Wc9PiN4+EJ4ZE
2GikYt4uPz2nitQBGkE63EMuLbStB6YASRdiZ3Wqf4rAvUqHIwqjv9WnVw+bJ9z7X4EiFB+JJQ7xrxls
g0+W49v7SP7VVcf9lTNh1zJvHz1O8/ufc7YMs6n1pvsKBdzQxkIjSWpGVLgOhF6G2uRh2/T0tSfQl1u0
uGDzH1b7dgeWF134qiz7TF2eb5MRXLvixfb+mcrKwWicn9n/2qm/dFdfiL8n2Rtcdc4/mAOUl45kN7Hx
/z2SrPt9ZNdMJDaec4EWaO0ei1FEl7+tjuuXtrQnC75yoz3TpamBo17O7JQCw48KGYoez1MGQ3dZl/Fv
5ncYhbg+J/ScwQiMbqql7i2xM9JOY4K+EXQwPfGmkjtadVaOrvaHehWanIPxP89zoOCC1Pt2J+fgB6IS
jNdz5yFrPg/ZnwAAAP//3oH4m7wIAAA=
`,
	},

	"/defs-image.json": {
		local:   "defs-image.json",
		size:    2916,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/8SWTU/bTBDH7/kU+xj0cMiLaVUhNUJIVbn0xIGeigIa1uN4qL3r7m6gIfJ3767t+D3Q
lKg9wc6u//P/zc443owY8wLUXFFqSApvzrxLDEmQW2mWgjLEVzEoZiS7SlF8lsIACVTsSwJLZNcpcgqJ
Q/74pNCrBKyeS2GDCQYEX9cpViEbpMAljIxJ9dz3pZXnW3k9k2rpax5hAj65VH4tMdkKmELQ00aRWNbx
FIxBlePc3nyafoPp8+n046L+97+j4/+nt3ez8WJzOnn3/izzf+/YsZenyIpMXkBL1KaJ1CmmiZBxtU6N
XCpII+LMEvHvepWw4lkmQ+YOyfsH5GbCSOTLEoCdnEego4v5eYQ/IbClTiAun7w42bMOBdLdeDZdjOd2
FTrWcYcoAUGhVb8sOaR6w4X1tXqOC+46rvDHihS6PDdlrNU9kzqo6bm1Li+jEUljMKFUiVeGFnVhlDVv
ext1A29Hm+661/ys49jeocIQlS0JBqyDlUsc23337JHfmJBGV1dnsy7k617cMdc792uDek8/1o2ePWgp
2sZImLMPw6Z6bf/vWv+FypYuBwlWKtav+AcWU2HSHWahkgl7shiRdUm6dM0SWLN7ZIF8ErG0NoO2s22b
g1Kwbm+RwaTrYfcol7uum8FV3hKQ19jLBjGrAeig7jfHlcog2lBrDU5xvgOKR5acm5XCLpzUTaJFS3HH
wPY1u7t/TOu/YLV/T63trA92OFtW7I1mZo82Q9HlhzNVib7VXIjgKn7YktWqO+31R7RIOTimr4I1J3II
9BEUgei+Q/eu10vF+ktgwy+hUfPv9uMChJAG2l+Ge99qU6T6PZcCr8LW22azx09dAul1jnrdAW6UezP0
7hOrOPZ60IvRdpWNstGvAAAA//83ffekZAsAAA==
`,
	},

	"/defs.json": {
		local:   "defs.json",
		size:    3193,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/7RWTXPaMBC98ys8tEfa2PIX9NYp/cghAzOZnjo9uGYBtSCpstxpmuG/VzLGWPZiMKWH
JPau9r23T6tYzwPHGS4gSyUVinI2fOMMp7CkjJq3zMkzWDhqLXm+WvNc6UdwZgLYO85UQhlI51FASpc0
TYry0R6vAtB4hkIHKVPj6k2/qycBhk3HYQWyqCwSW127zbc698oj42M4+V2GPRIXwd2oQvaivtA+iSMM
3MRb8D7pC0+8IA7GfhRgHFWyRRQFfYkmhPh+TFw/GodBHEeu6yKMyCqLOr9idzAeEoYt3N57gwFHYei3
oXvvCwYdkEkwiWIyaeP33g4M3xsHQRQHgRv7sTsJQ4KZ70VzbnBlnZAzmC114EsZcKpUkX4pwWSHL+5q
B+6utLxauBvh1YduWL7Z1FaXT18RL26pUDt7U4WZkpSt+is8cOzrb6tpm4jHAnb/Gxsl/u07pOo4SSJR
Wj+bSy5AKgpZrUinXz97o50V6mphUP/bEjXbU/9nUSXWGVGf76d1IafHRh94q/DjtYVvpUyeZktdn2EW
JCZ9dIAq2Da6xqmMHrTDkm9v/ZWU+EbbPB/oBuaJWmMM9brD+vfs13kDG+ItgE+c/6gjiBNTImxRHWxV
C9hh1DatsstwMNVNNLDa7wAzPp0Z4pLPGHLTmSocRhnvpw+JEI1/Lac2YM0zZZ2WDsr6iWlalh5ufrcA
y+gfuBIFdeSB50xd4kbGc5leSN09kPryrChLysvzP8NxYd+bO6EuGfFy/Pp9MqoplfAzpxIW1hfU6nnU
MrVJbn8cB+ZnN/gbAAD//0JyEpx5DAAA
`,
	},

	"/image-layout-schema.json": {
		local:   "image-layout-schema.json",
		size:    414,
		modtime: 1485388791,
		compressed: `
H4sIAAAJbogA/2yPsU7EMAyG9z6FFRhpUySmW286CekGJBbEEFpfmxNNQuIinVDfHSduYbhbWvmPP3/2
TwWgekxdtIGsd2oH6hjQ7b0jYx1GOExmQHg2Fz8TvHQjTkY9ZOo+ScHESBR2Wp+Td7WkjY+D7qM5Ud0+
acnuhLP9hiRmPMu6TZYKJt3aZnH9WcRC0iVgZv3HGbs1C5EnRLKY+CVfkw2ZlI1feaicJW/X135LB/gT
0Ihw3B/gyly4zZ4oWjf85+jmifO3tebksWmbVq31e/kv/F3KwhG/Zhux/0NurVtlbql+AwAA//8bwMuB
ngEAAA==
`,
	},

	"/image-manifest-schema.json": {
		local:   "image-manifest-schema.json",
		size:    921,
		modtime: 1485389045,
		compressed: `
H4sIAAAJbogA/5ySMU/rMBSF9/yKq7RjU7/39KauTB0QA4gFMZjkJrlVbQfbRVSo/51ru6Y1ZUAdc+xz
7ndP/FEB1B261tLkyeh6BfXdhPrGaC9Jo4W1kgPCrdTUo/NwP2FLPbUy3l4E+9y1IyoZrKP300qIjTO6
SerS2EF0Vva++fNfJG2WfNRli2OP4altnuqiLd0WFAiEOhIkr99PGNzmZYPtUZssZ1hP6PgkLMZainjk
xLRcki93fhjJQU+47cClDdGBHxHicMjDIeXBWwoE6UBqIO1xQBspYvh1m4kS9ist73oxRpEmtVN89u+k
yfesRemQTmoG6Gk4b2BusQ+xAQ21b3Ijxi7D/6sL+1buGevcnqmktXJfMK09qnD176mPo5LNv53O8wsK
qbXx8eUVKNfV3WyJOz+PXHyvpsPeNdEVoWaCBe483i6cVWaNpLXF1x1ZDFhPP73D8p+UFfPHc3WoPgMA
AP//UcoRdpkDAAA=
`,
	},

	"/manifest-list-schema.json": {
		local:   "manifest-list-schema.json",
		size:    873,
		modtime: 1485389045,
		compressed: `
H4sIAAAJbogA/6SSv0/7MBDF9/wVp/Q7flMjxNQVFiQQA4gFMZjk0lzV2OHORVSo/zv+EZdEZUDqUqnP
fu8+7+KvAqBsUGqmwZE15QrKhwHNtTVOk0GG216vEe61oRbFwR35n8cBa2qp1tHyP2T8k7rDXgd/59yw
Umoj1lRJXVpeq4Z166qLK5W0RfJRky3iPdaPrvNoibZ0W1HAUP2IUW09Rgpw+wFDhH3bYD1qA/sgdoTi
T0JFr6WcZx+baib5tP1TRwIt4bYBSTVRwHUIkQBmBJBC4SOlghbQBsg4XCNHlDjhjI5qjn2MzK1PZvVk
qN/1/uzyR9OfWYvSIZ2UeZJM15GTNbPeTzo47Kf3widnbMPNBlupIvsyfPOF8oKnCAuVY5ubccuWyzHh
MGPRxlgX39OM5pzVTSOPPf4EPXUWmTWSlozvO2IMWC+/PayT1fr/r8Wh+A4AAP//b2/SMmkDAAA=
`,
	},

	"/": {
		isDir: true,
		local: "/",
	},
}
