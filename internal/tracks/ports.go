package tracks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// portBlock is how many ports each track's block has.
const portBlock = 10

// portFor is the port d starts on in track t: the next free one of the
// track's block, its fixed port, or none to detect. used are ports
// already taken in the track.
func (s *Service) portFor(ctx context.Context, t track.Track, d devServer, used map[int]bool) (int, error) {
	free := s.PortFree
	if free == nil {
		free = portFree
	}
	switch d.def.PortMode {
	case store.PortDetect:
		return 0, nil
	case store.PortFixed:
		if used[d.def.Port] {
			return 0, Problem(fmt.Sprintf("%s needs port %d, which another server of this track uses.", d.def.Name, d.def.Port))
		}
		if holder, err := s.holderOf(d.def.Port, t.ID); err != nil {
			return 0, err
		} else if holder != "" {
			return 0, Problem(fmt.Sprintf("%s needs port %d, which %s uses.", d.def.Name, d.def.Port, holder))
		}
		if !free(d.def.Port) {
			return 0, Problem(fmt.Sprintf("%s needs port %d, which another program uses.", d.def.Name, d.def.Port))
		}
		return d.def.Port, nil
	}
	blocks := (track.LastAssignedPort - track.FirstAssignedPort + 1) / portBlock
	base, err := s.Store.ClaimPorts(ctx, t.ID, track.FirstAssignedPort, portBlock, blocks)
	if errors.Is(err, store.ErrNoPorts) {
		return 0, Problem("Every block of ports is taken by open tracks; end some first.")
	} else if err != nil {
		return 0, err
	}
	for p := base; p < base+portBlock; p++ {
		if !used[p] && free(p) {
			return p, nil
		}
	}
	return 0, Problem(fmt.Sprintf("Every port of this track, %d–%d, is taken.", base, base+portBlock-1))
}

// holderOf names the dev server running on port in another track than
// id, as "api/metro in fix-login", or "" for none.
func (s *Service) holderOf(port int, id string) (string, error) {
	infos, err := s.Windows.List()
	if err != nil {
		return "", err
	}
	for _, in := range infos {
		if in.Track == "" || in.Track == id {
			continue
		}
		panes, err := s.Servers.Panes(in.Window)
		if err != nil {
			continue
		}
		for _, p := range panes {
			if p.Role == trackwin.RoleDevServer && p.Port == port && serverState(p) == ServerRunning {
				return p.Key + " in " + in.Name, nil
			}
		}
	}
	return "", nil
}

// portFree reports whether nothing listens on port. Each loopback is
// tried on its own: on macOS a wildcard bind succeeds next to a server
// on 127.0.0.1 or ::1, which is where dev servers usually listen. An
// address this machine lacks, such as ::1 without IPv6, doesn't count.
func portFree(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1", ""} {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if errors.Is(err, syscall.EADDRINUSE) {
			return false
		}
		if err == nil {
			_ = ln.Close()
		}
	}
	return true
}
