package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/spf13/cobra"
)

// maxStreamLine bounds one event of a reviewer's stream: a tool call's
// result can hold a whole file.
const maxStreamLine = 64 << 20

func newReviewStreamCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:    "review-stream",
		Hidden: true,
		Short:  "show a Cursor reviewer's streamed turn, and keep its final text",
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if out == "" {
				return errors.New("--out is required")
			}
			return followStream(c.InOrStdin(), c.OutOrStdout(), out)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "the file the turn's final text goes to")
	return cmd
}

// followStream shows a stream-json turn read from in on shown as it
// comes, and writes its final text to out, whole: a turn that failed or
// never ended writes nothing, and its hook says so.
func followStream(in io.Reader, shown io.Writer, out string) error {
	// An earlier turn's text, such as the last question's answer, must
	// not pass for this one's.
	if err := os.Remove(out); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var s cursor.Stream
	r := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := readLine(r)
		if line != "" {
			fmt.Fprint(shown, s.Feed(line))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if !s.Done || s.Failed {
		return nil
	}
	if err := os.WriteFile(out+".part", []byte(s.Result), 0o600); err != nil {
		return err
	}
	if err := os.Rename(out+".part", out); err != nil {
		_ = os.Remove(out + ".part")
		return err
	}
	return nil
}

// readLine is the next line of r without its end, at most maxStreamLine
// bytes of it.
func readLine(r *bufio.Reader) (string, error) {
	var line []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		if len(line) < maxStreamLine {
			line = append(line, chunk...)
		}
		if err != nil || !isPrefix {
			return string(line), err
		}
	}
}
