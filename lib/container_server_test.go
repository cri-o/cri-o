package lib

import (
	"testing"
)

func TestLocaleToLanguage(t *testing.T) {
	for _, testCase := range []struct {
		locale   string
		language string
	}{
		{
			locale:   "",
			language: "und-u-va-posix",
		},
		{
			locale:   "C",
			language: "und-u-va-posix",
		},
		{
			locale:   "POSIX",
			language: "und-u-va-posix",
		},
		{
			locale:   "c",
			language: "und-u-va-posix",
		},
		{
			locale:   "en",
			language: "en",
		},
		{
			locale:   "en_US",
			language: "en-US",
		},
		{
			locale:   "en.UTF-8",
			language: "en",
		},
		{
			locale:   "en_US.UTF-8",
			language: "en-US",
		},
		{
			locale:   "does-not-exist",
			language: "does-not-exist",
		},
	} {
		t.Run(testCase.locale, func(t *testing.T) {
			language := localeToLanguage(testCase.locale)
			if language != testCase.language {
				t.Fatalf("%q converted to %q, but expected %q", testCase.locale, language, testCase.language)
			}
		})
	}
}
