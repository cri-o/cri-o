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

package notes

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var headerPattern = regexp.MustCompile("^(?P<indent>#+) ?(?P<title>.+)$")

func GenerateTOC(input string) (string, error) {
	result := &strings.Builder{}

	lastLen, indent := 0, 0
	headers := map[string]int{}
	seenBackTicks := 0

	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		// skip code blocks if necessary
		seenBackTicks += strings.Count(scanner.Text(), "`")
		if seenBackTicks%2 != 0 {
			continue
		}

		if headerPattern.Match(scanner.Bytes()) {
			matches := headerPattern.FindStringSubmatch(scanner.Text())

			i := len(matches[1])
			if i == 1 {
				indent = 1
			} else if i > lastLen {
				indent++
			} else if i < lastLen {
				indent--
			}
			lastLen = i

			add(result, matches[2], indent-1, headers)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return result.String(), nil
}

func add(result io.StringWriter, title string, indent int, headers map[string]int) {
	link := strings.NewReplacer(
		"!", "",
		"#", "",
		"%", "",
		"&", "",
		"'", "",
		"(", "",
		")", "",
		"*", "",
		",", "",
		".", "",
		"@", "",
		"[", "",
		"\"", "",
		"]", "",
		"^", "",
		"`", "",
		"{", "",
		"|", "",
		"}", "",
		"~", "",
		" ", "-",
	).Replace(strings.ToLower(title))

	if _, ok := headers[link]; ok {
		headers[link]++
		link = fmt.Sprintf("%s-%d", link, headers[link]-1)
	} else {
		headers[link] = 1
	}

	result.WriteString(fmt.Sprintf( // nolint: errcheck
		"%s- [%s](#%s)\n", strings.Repeat(" ", indent*2), title, link),
	)
}
