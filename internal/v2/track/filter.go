package track

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Started is when a Filter's tracks started.
type Started string

// The Started presets. Between reads Filter's From and To.
const (
	AnyTime    Started = ""
	Today      Started = "today"
	Last7Days  Started = "7d"
	Last30Days Started = "30d"
	Between    Started = "between"
)

// DateLayout is how Filter's From and To are written.
const DateLayout = "2006-01-02"

// NoPRLabel is the filter's name for NoPRs.
const NoPRLabel = "no PR"

// Filter picks the tracks Station lists; the zero Filter picks the usual
// ones. Statuses and PRStatuses are status IDs: none in a group means
// all of it. Archived lists the archived tracks instead of the others.
// From and To are dates in DateLayout, both included, in local time;
// either may be empty.
type Filter struct {
	Statuses   []string `json:"statuses,omitempty"`
	PRStatuses []string `json:"pr_statuses,omitempty"`
	Archived   bool     `json:"archived,omitempty"`
	Started    Started  `json:"started,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
}

// On reports whether f picks anything but the usual tracks.
func (f Filter) On() bool {
	return len(f.Statuses) > 0 || len(f.PRStatuses) > 0 || f.Archived || f.Started != AnyTime
}

// Check says what's wrong with f, worded for the user.
func (f Filter) Check() error {
	for _, id := range f.Statuses {
		if !slices.ContainsFunc(Statuses, func(s Status) bool { return s.ID == id }) {
			return errors.New("There's no track status " + id + ".")
		}
	}
	for _, id := range f.PRStatuses {
		if PRFilterLabel(id) == "" {
			return errors.New("There's no PR status " + id + ".")
		}
	}
	switch f.Started {
	case AnyTime, Today, Last7Days, Last30Days:
		return nil
	case Between:
	default:
		return errors.New("There's no start " + string(f.Started) + ".")
	}
	var from, to time.Time
	var err error
	if f.From != "" {
		if from, err = time.Parse(DateLayout, f.From); err != nil {
			return errors.New("From isn't a date like 2026-09-29.")
		}
	}
	if f.To != "" {
		if to, err = time.Parse(DateLayout, f.To); err != nil {
			return errors.New("To isn't a date like 2026-09-29.")
		}
	}
	switch {
	case f.From == "" && f.To == "":
		return errors.New("Enter a From or a To date.")
	case f.From != "" && f.To != "" && from.After(to):
		return errors.New("From is after To.")
	}
	return nil
}

// Range is when f's tracks started, in now's location: from included,
// to not; a zero end is open.
func (f Filter) Range(now time.Time) (from, to time.Time) {
	loc := now.Location()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	switch f.Started {
	case Today:
		return midnight, time.Time{}
	case Last7Days:
		return midnight.AddDate(0, 0, -6), time.Time{}
	case Last30Days:
		return midnight.AddDate(0, 0, -29), time.Time{}
	case Between:
		if d, err := time.ParseInLocation(DateLayout, f.From, loc); err == nil {
			from = d
		}
		if d, err := time.ParseInLocation(DateLayout, f.To, loc); err == nil {
			to = d.AddDate(0, 0, 1)
		}
	}
	return from, to
}

// Match reports whether f picks t at now.
func (f Filter) Match(t Track, now time.Time) bool {
	if t.Archived() != f.Archived {
		return false
	}
	if len(f.Statuses) > 0 && !slices.Contains(f.Statuses, t.Status().ID) {
		return false
	}
	if len(f.PRStatuses) > 0 && !slices.Contains(f.PRStatuses, PRStatus(t.PRs).ID) {
		return false
	}
	from, to := f.Range(now)
	return (from.IsZero() || !t.CreatedAt.Before(from)) && (to.IsZero() || t.CreatedAt.Before(to))
}

// String is f as Station's Filtered line words it, such as
// "done, closed · PR merged · started in the last 7 days".
func (f Filter) String() string {
	var parts []string
	if f.Archived {
		parts = append(parts, "archived")
	}
	if len(f.Statuses) > 0 {
		var labels []string
		for _, s := range Statuses {
			if slices.Contains(f.Statuses, s.ID) {
				labels = append(labels, s.Label)
			}
		}
		parts = append(parts, strings.Join(labels, ", "))
	}
	if len(f.PRStatuses) > 0 {
		var labels []string
		for _, id := range FilterPRStatuses {
			if slices.Contains(f.PRStatuses, id) {
				labels = append(labels, PRFilterLabel(id))
			}
		}
		parts = append(parts, strings.Join(labels, ", "))
	}
	switch f.Started {
	case Today:
		parts = append(parts, "started today")
	case Last7Days:
		parts = append(parts, "started in the last 7 days")
	case Last30Days:
		parts = append(parts, "started in the last 30 days")
	case Between:
		switch {
		case f.From == "":
			parts = append(parts, "started until "+f.To)
		case f.To == "":
			parts = append(parts, "started from "+f.From)
		default:
			parts = append(parts, "started "+f.From+" to "+f.To)
		}
	}
	return strings.Join(parts, " · ")
}

// FilterPRStatuses are the PR status IDs a filter picks from, in order.
var FilterPRStatuses = []string{NoPRs.ID, PRsOpen.ID, PRsMerged.ID, PRsClosed.ID}

// PRFilterLabel is how the filter names PR status id; "" for none.
func PRFilterLabel(id string) string {
	if id == NoPRs.ID {
		return NoPRLabel
	}
	for _, s := range PRStatuses {
		if s.ID == id {
			return s.Label
		}
	}
	return ""
}
