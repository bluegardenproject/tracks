package widget

import "testing"

func TestPickerFilter(t *testing.T) {
	p := NewPicker("Model", []PickerItem{{Label: "Default"}, {Label: "gpt-5", Detail: "Codex"}, {Label: "opus"}, {Label: "gpt-4", Detail: "Codex"}}, 0)
	p.Filter = true
	p.Size(80, 20)
	for _, k := range []string{"c", "o", "d", "x", "backspace"} {
		p.Key(k)
	}
	if p.Cursor != 1 {
		t.Fatalf("typing \"cod\" put the cursor on %d; want the first match, 1", p.Cursor)
	}
	p.Key("down")
	if p.Key("enter") != PickerChosen || p.Cursor != 3 {
		t.Errorf("down, enter chose %d; want the next match, 3", p.Cursor)
	}
	if p.Key("j") != PickerOpen || p.query != "codj" {
		t.Errorf("j should type into the filter, got %q", p.query)
	}
	if p.Key("enter") != PickerOpen {
		t.Error("Enter with nothing shown shouldn't choose")
	}

	p.Message, p.Problem = "the model list took too long", true
	if p.Key("enter") != PickerRetry || p.Key("esc") != PickerClosed {
		t.Error("an error message should retry on Enter and close on Esc")
	}
}
