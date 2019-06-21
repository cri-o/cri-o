package storage_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
)

type mockSequence struct {
	first, last *gomock.Call // may be both nil (= the default value of mockSequence) to mean empty sequence
}

// like gomock.inOrder, but can be nested
func inOrder(calls ...interface{}) mockSequence {
	var first, last *gomock.Call
	// This implementation does a few more assignments and checks than strictly necessary, but it is O(N) and reasonably easy to read, so, whatever.
	for i := 0; i < len(calls); i++ {
		var elem mockSequence
		switch e := calls[i].(type) {
		case mockSequence:
			elem = e
		case *gomock.Call:
			elem = mockSequence{e, e}
		default:
			Fail(fmt.Sprintf("Invalid inOrder parameter %#v", e))
		}

		if elem.first == nil {
			continue
		}
		if first == nil {
			first = elem.first
		} else if last != nil {
			elem.first.After(last)
		}
		last = elem.last
	}
	return mockSequence{first, last}
}
