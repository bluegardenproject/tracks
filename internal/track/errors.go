package track

// Error kinds: a dev server that crashed, or a setup that failed.
const (
	ServerError = "server"
	SetupError  = "setup"
)

// Failure is a track's server or setup error, shown until it's cleared.
type Failure struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"` // repo/server for a server, the repo for a setup
	Code    int    `json:"code"`
}

// The error statuses, from a track's errors.
var (
	NoErrors      = Status{ID: "none"}
	ServerErrored = Status{ID: "server-error", Label: "server error", Badge: BadgeDanger}
	SetupErrored  = Status{ID: "setup-error", Label: "setup error", Badge: BadgeDanger}
	BothErrored   = Status{ID: "errors", Label: "server & setup error", Badge: BadgeDanger}
)

// FailureStatus is the badge for failures.
func FailureStatus(failures []Failure) Status {
	server, setup := false, false
	for _, e := range failures {
		server = server || e.Kind == ServerError
		setup = setup || e.Kind == SetupError
	}
	switch {
	case server && setup:
		return BothErrored
	case server:
		return ServerErrored
	case setup:
		return SetupErrored
	}
	return NoErrors
}
