package main

// newAppEnv marks a process as part of the Tracks v2 app. The v2 app
// sets it for everything it starts, so child processes stay in v2
// without repeating the flag.
const newAppEnv = "TRACKS_NEW_APP"

// newAppRequested reports whether this run belongs to the v2 app
// (--new-app, or TRACKS_NEW_APP=1) and returns args without the flag.
// Arguments after "--" are left alone.
func newAppRequested(args []string, getenv func(string) string) (bool, []string) {
	requested := getenv(newAppEnv) == "1"
	rest := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if a == "--new-app" {
			requested = true
			continue
		}
		rest = append(rest, a)
	}
	return requested, rest
}
