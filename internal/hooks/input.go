package hooks

import (
	"regexp"
	"strings"
	"unicode"
)

// InputEmpty reports whether screen, an engine's agent pane captured
// with its styles, shows its input empty: nothing after the prompt
// marker but the placeholder, which both engines draw dim. known is
// false when the input isn't found, such as while a dialog covers it.
func InputEmpty(engine, screen string) (empty, known bool) {
	lines := strings.Split(screen, "\n")
	switch engine {
	case "claude":
		return claudeInputEmpty(lines)
	case "cursor":
		return cursorInputEmpty(lines)
	}
	return false, false
}

// claudeInputEmpty reads Claude's input: the lines between its two
// rules, the first starting with ❯.
func claudeInputEmpty(lines []string) (empty, known bool) {
	var rules []int
	for i, l := range lines {
		if isRule(plain(l)) {
			rules = append(rules, i)
		}
	}
	for r := len(rules) - 1; r > 0; r-- {
		top, bottom := rules[r-1], rules[r]
		if top+1 >= bottom {
			continue
		}
		first := cells(lines[top+1])
		at := markerAt(first, '❯')
		if at < 0 {
			continue
		}
		input := [][]cell{first[at+1:]}
		for _, l := range lines[top+2 : bottom] {
			input = append(input, cells(l))
		}
		return allDim(input), true
	}
	return false, false
}

// cursorInputEmpty reads Cursor's input: the last line starting with →,
// which it draws dim, with its placeholder, while the input is empty.
func cursorInputEmpty(lines []string) (empty, known bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		c := cells(lines[i])
		at := markerAt(c, '→')
		if at < 0 {
			continue
		}
		return allDim([][]cell{c[at:]}), true
	}
	return false, false
}

var rule = regexp.MustCompile(`^─{10,}$`)

func isRule(line string) bool { return rule.MatchString(strings.TrimSpace(line)) }

// cell is a character on screen and whether it's drawn dim.
type cell struct {
	r   rune
	dim bool
}

// cells are line's characters and their dimness, its escapes read.
func cells(line string) []cell {
	var out []cell
	dim := false
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		if rs[i] != '\x1b' {
			out = append(out, cell{rs[i], dim})
			continue
		}
		if i+1 >= len(rs) || rs[i+1] != '[' {
			// Not a CSI sequence, such as OSC: skip to its end.
			for i++; i < len(rs) && rs[i] != '\a' && rs[i] != '\\'; i++ {
			}
			continue
		}
		start := i + 2
		end := start
		for end < len(rs) && (rs[end] < 0x40 || rs[end] > 0x7e) {
			end++
		}
		if end < len(rs) && rs[end] == 'm' {
			dim = sgrDim(string(rs[start:end]), dim)
		}
		i = end
	}
	return out
}

// sgrDim is whether text is dim after the SGR parameters params, dim
// before them. Colours' own numbers, as in 48;2;21;21;21, aren't
// attributes.
func sgrDim(params string, dim bool) bool {
	if params == "" {
		return false
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		switch ps[i] {
		case "0", "":
			dim = false
		case "2":
			dim = true
		case "22":
			dim = false
		case "38", "48", "58":
			if i+1 < len(ps) && ps[i+1] == "5" {
				i += 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				i += 4
			}
		}
	}
	return dim
}

// markerAt is where marker is in c, the first character other than
// spaces and box lines; -1 when that's another.
func markerAt(c []cell, marker rune) int {
	for i, x := range c {
		switch {
		case x.r == marker:
			return i
		case unicode.IsSpace(x.r) || x.r == '│':
		default:
			return -1
		}
	}
	return -1
}

// allDim reports whether every character of lines that isn't a space or
// a box line is dim.
func allDim(lines [][]cell) bool {
	for _, l := range lines {
		for _, x := range l {
			if !x.dim && !unicode.IsSpace(x.r) && x.r != '│' && x.r != '→' && x.r != '❯' {
				return false
			}
		}
	}
	return true
}

// plain is line without its escapes.
func plain(line string) string {
	var b strings.Builder
	for _, c := range cells(line) {
		b.WriteRune(c.r)
	}
	return b.String()
}
