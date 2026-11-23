package main

import (
	"context"
	"testing"
)

func TestNewLibraryRequestCancelsPrevious(t *testing.T) {
	root, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	u := &UI{ctx: root}
	first, cancelFirst, g1 := u.begin("first")
	defer cancelFirst()
	next, cancelNext, g2 := u.begin("next")
	defer cancelNext()
	if first.Err() != context.Canceled {
		t.Fatal("previous library fetch still active")
	}
	if next.Err() != nil || g2 <= g1 {
		t.Fatal("new request canceled")
	}
	u.cancelLibrary()
	if next.Err() != context.Canceled {
		t.Fatal("logout cancellation failed")
	}
}
