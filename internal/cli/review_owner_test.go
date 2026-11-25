package cli

import "testing"

func TestReviewOwnerFlag(t *testing.T) {
	for _, c := range []struct {
		args  []string
		owner string
		ok    bool
	}{
		{nil, "", true},
		{[]string{"--owner"}, repoOwner, true},
		{[]string{"--owner", "@org/team"}, "@org/team", true},
		{[]string{"--owner=@org/team"}, "@org/team", true},
		{[]string{"stray"}, "", false},
		{[]string{"--owner=@org/team", "stray"}, "@org/team", false},
	} {
		cmd := newReviewCmd()
		if err := cmd.ParseFlags(c.args); err != nil {
			t.Fatalf("%q: %v", c.args, err)
		}
		err := cmd.ValidateArgs(cmd.Flags().Args())
		if got, _ := cmd.Flags().GetString("owner"); (err == nil) != c.ok || err == nil && got != c.owner {
			t.Errorf("%q: owner %q, err %v; want %q, ok %v", c.args, got, err, c.owner, c.ok)
		}
	}
}
