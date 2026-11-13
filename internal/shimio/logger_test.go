package shimio_test

import (
	"bytes"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/shimio"
)

var _ = Describe("NewCRILogger", func() {
	// bufSize matches the logger's internal read buffer size.
	const bufSize = 4096

	full, partial := types.LogTagFull, types.LogTagPartial

	DescribeTable(
		"should write CRI formatted log lines",
		func(input string, stream shimio.StreamType, tags []types.LogTag, content []string) {
			var buf bytes.Buffer

			w, stopped := shimio.NewCRILogger("test-path", &buf, stream)
			_, err := io.WriteString(w, input)
			Expect(err).NotTo(HaveOccurred())
			Expect(w.Close()).To(Succeed())
			Eventually(stopped).Should(BeClosed())

			lines := strings.Split(buf.String(), "\n")
			lines = lines[:len(lines)-1] // Discard empty string after last \n
			Expect(lines).To(HaveLen(len(content)))

			for i := range lines {
				fields := strings.SplitN(lines[i], " ", 4)
				Expect(fields).To(HaveLen(4))

				_, err := time.Parse(time.RFC3339Nano, fields[0])
				Expect(err).NotTo(HaveOccurred())
				Expect(fields[1]).To(Equal(string(stream)))
				Expect(fields[2]).To(Equal(string(tags[i])))
				Expect(fields[3]).To(Equal(content[i]))
			}
		},
		Entry("stdout log",
			"log 1\nlog 2\n", shimio.Stdout,
			[]types.LogTag{full, full}, []string{"log 1", "log 2"},
		),
		Entry("stderr log",
			"log 1\nlog 2\n", shimio.Stderr,
			[]types.LogTag{full, full}, []string{"log 1", "log 2"},
		),
		Entry("log ends without newline",
			"log 1\nlog 2", shimio.Stderr,
			[]types.LogTag{full, partial}, []string{"log 1", "log 2"},
		),
		Entry("empty input",
			"", shimio.Stdout,
			[]types.LogTag{}, []string{},
		),
		Entry("log length longer than buffer size is not split",
			strings.Repeat("a", bufSize*10+10)+"\n", shimio.Stdout,
			[]types.LogTag{full}, []string{strings.Repeat("a", bufSize*10+10)},
		),
		Entry("long log without newline is a single partial line",
			strings.Repeat("a", bufSize*2+10), shimio.Stdout,
			[]types.LogTag{partial}, []string{strings.Repeat("a", bufSize*2+10)},
		),
		Entry("trailing \\r\\n straddling the buffer",
			strings.Repeat("a", bufSize-1)+"\r\n", shimio.Stdout,
			[]types.LogTag{full}, []string{strings.Repeat("a", bufSize-1)},
		),
	)
})
