package git

import "testing"

func TestParseShortStatAllThreeClauses(t *testing.T) {
	got := parseShortStat(" 5 files changed, 120 insertions(+), 30 deletions(-)")
	want := ShortStat{Files: 5, Insertions: 120, Deletions: 30}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestParseShortStatSingularFile(t *testing.T) {
	got := parseShortStat(" 1 file changed, 2 insertions(+), 1 deletion(-)")
	want := ShortStat{Files: 1, Insertions: 2, Deletions: 1}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestParseShortStatNoInsertions(t *testing.T) {
	got := parseShortStat(" 3 files changed, 25 deletions(-)")
	want := ShortStat{Files: 3, Deletions: 25}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestParseShortStatNoDeletions(t *testing.T) {
	got := parseShortStat(" 2 files changed, 7 insertions(+)")
	want := ShortStat{Files: 2, Insertions: 7}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestParseShortStatEmpty(t *testing.T) {
	got := parseShortStat("")
	if got != (ShortStat{}) {
		t.Errorf("got %+v want zero", got)
	}
}
