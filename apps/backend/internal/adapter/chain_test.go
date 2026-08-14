package adapter

import "testing"

func TestPageStatusIsIncompleteWhenAPageHasMore(t *testing.T) {
	if PageStatus(SourceStatus{IsComplete: true}, true).IsComplete {
		t.Fatal("paged transfer history was marked complete")
	}
	if !PageStatus(SourceStatus{IsComplete: true}, false).IsComplete {
		t.Fatal("complete final page was changed")
	}
}
