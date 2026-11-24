package tmux

import (
	"testing"
	"time"
)

func TestLastKey(t *testing.T) {
	out := "@1\t1795447953\n@2\t1795447999\n@1\t1795447960\n@1\tjunk\nnot a line"
	if got := lastKey(out, "@1"); !got.Equal(time.Unix(1795447960, 0)) {
		t.Errorf("lastKey(@1) = %v, want the later of its two clients", got)
	}
	if got := lastKey(out, "@3"); !got.IsZero() {
		t.Errorf("a window no client shows: %v", got)
	}
}
