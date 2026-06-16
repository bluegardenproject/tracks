package daemon

import "testing"

func TestParseReviewRef(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantFetchRef string
		wantLabel    string
		wantErr      bool
	}{
		{
			name:         "pr url",
			in:           "https://github.com/org/repo/pull/123",
			wantFetchRef: "pull/123/head",
			wantLabel:    "pr/123",
		},
		{
			name:         "pr url with files suffix",
			in:           "https://github.com/org/repo/pull/42/files",
			wantFetchRef: "pull/42/head",
			wantLabel:    "pr/42",
		},
		{
			name:         "branch name",
			in:           "feat/foo",
			wantFetchRef: "feat/foo",
			wantLabel:    "feat/foo",
		},
		{
			name:         "branch name trimmed",
			in:           "  fix/bar  ",
			wantFetchRef: "fix/bar",
			wantLabel:    "fix/bar",
		},
		{
			name:    "empty",
			in:      "   ",
			wantErr: true,
		},
		{
			name:    "github url that is not a PR",
			in:      "https://github.com/org/repo/tree/main",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReviewRef(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseReviewRef(%q) = %+v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseReviewRef(%q): unexpected error %v", tc.in, err)
			}
			if got.fetchRef != tc.wantFetchRef {
				t.Errorf("fetchRef = %q, want %q", got.fetchRef, tc.wantFetchRef)
			}
			if got.label != tc.wantLabel {
				t.Errorf("label = %q, want %q", got.label, tc.wantLabel)
			}
		})
	}
}
