package track

// Dev servers whose port Tracks assigns get one from this range. Fixed
// ports and the proxy's ports stay out of it.
const (
	FirstAssignedPort = 20000
	LastAssignedPort  = 29999
)
