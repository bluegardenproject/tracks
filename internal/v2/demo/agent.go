package demo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"time"
)

// Scenario is what the fake agent acts out.
type Scenario string

const (
	Implementing Scenario = "implementing"
	Question     Scenario = "question"
	Review       Scenario = "review"
)

type step struct {
	pause time.Duration
	line  string
}

var scripts = map[Scenario][]step{
	Implementing: {
		{0, "> Add token refresh to the auth middleware"},
		{800 * time.Millisecond, ""},
		{600 * time.Millisecond, "● Reading internal/auth/middleware.go"},
		{900 * time.Millisecond, "● Reading internal/auth/token.go"},
		{1200 * time.Millisecond, "● Editing internal/auth/middleware.go (+42 −7)"},
		{1500 * time.Millisecond, "● Running go test ./internal/auth/..."},
		{2000 * time.Millisecond, "  ok  shop-api/internal/auth  0.412s"},
		{800 * time.Millisecond, ""},
		{400 * time.Millisecond, "Tokens now refresh 60 seconds before they expire. Tests pass."},
	},
	Question: {
		{0, "> Fix the flicker when the checkout total updates"},
		{800 * time.Millisecond, ""},
		{600 * time.Millisecond, "● Reading src/checkout/Total.tsx"},
		{1000 * time.Millisecond, "● Searching for useCartTotal"},
		{1200 * time.Millisecond, ""},
		{300 * time.Millisecond, "The total re-renders on every cart event. Two ways to fix it:"},
		{300 * time.Millisecond, "  1. memoise the total in useCartTotal"},
		{300 * time.Millisecond, "  2. debounce cart events by 50 ms"},
		{300 * time.Millisecond, "Which do you prefer?"},
	},
	Review: {
		{0, "> Review pull request #1423"},
		{800 * time.Millisecond, ""},
		{700 * time.Millisecond, "● Reading the diff (12 files, +310 −95)"},
		{1500 * time.Millisecond, "● Running the test suite"},
		{2000 * time.Millisecond, ""},
		{300 * time.Millisecond, "Two findings:"},
		{300 * time.Millisecond, "  - src/cart/api.ts:88 swallows the error from fetchCart"},
		{300 * time.Millisecond, "  - the new banner has no dark-mode colours"},
		{300 * time.Millisecond, "Otherwise ready to merge."},
	},
}

// RunAgent plays scenario to w, then answers whatever is typed on in
// until in closes or ctx ends.
func RunAgent(ctx context.Context, w io.Writer, in io.Reader, scenario Scenario, track string) error {
	script, ok := scripts[scenario]
	if !ok {
		return fmt.Errorf("unknown scenario %q", scenario)
	}
	fmt.Fprintf(w, "Demo agent · %s\n\n", track)
	for _, s := range script {
		if err := sleep(ctx, s.pause); err != nil {
			return nil
		}
		fmt.Fprintln(w, s.line)
	}

	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	for {
		fmt.Fprint(w, "\n> ")
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-lines:
			if !ok {
				return nil
			}
			fmt.Fprintln(w, "This is a demo agent, so nothing happens.")
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d == 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
