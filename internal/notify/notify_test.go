package notify

import "testing"

func TestQuoteEscapesDoubleQuotes(t *testing.T) {
	got := quote(`he said "hi"`)
	want := `"he said \"hi\""`
	if got != want {
		t.Errorf("quote: got %q want %q", got, want)
	}
}

func TestSendNilNotifierIsNoop(t *testing.T) {
	var n *Notifier
	// Should not panic.
	n.Send("title", "body")
}

func TestSendBothChannelsDisabledIsNoop(t *testing.T) {
	n := New(Channel{})
	// Should not panic and should not contact /dev/tty or osascript.
	// We have no way to assert non-execution here, but it must not
	// crash on any platform.
	n.Send("title", "body")
}
