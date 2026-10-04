package repos

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/store"
)

func TestAddChecksDevServers(t *testing.T) {
	ctx := context.Background()
	dir := clone(t, "shop")
	s := service(t)
	web := store.DevServer{Name: "web", Command: "pnpm dev"}
	for _, c := range []struct {
		setup   string
		servers []store.DevServer
		field   string
	}{
		{"pnpm install\npnpm build", nil, "setup"},
		{"", []store.DevServer{{Name: " ", Command: "pnpm dev"}}, "servers.0.name"},
		{"", []store.DevServer{{Name: strings.Repeat("w", 33), Command: "pnpm dev"}}, "servers.0.name"},
		{"", []store.DevServer{web, {Name: "WEB", Command: "pnpm dev"}}, "servers.1.name"},
		{"", []store.DevServer{{Name: "web", Command: "  "}}, "servers.0.command"},
		{"", []store.DevServer{{Name: "web", Command: "a\nb"}}, "servers.0.command"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", Dir: "/abs"}}, "servers.0.dir"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", Dir: "apps/../.."}}, "servers.0.dir"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", Dir: "apps/\x1bweb"}}, "servers.0.dir"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm\x00dev"}}, "servers.0.command"},
		{"pnpm\tinstall", nil, "setup"},
		{"", []store.DevServer{{Name: "a", Command: "x", PortMode: store.PortFixed, Port: 8081},
			{Name: "b", Command: "y", PortMode: store.PortFixed, Port: 8081}}, "servers.1.port"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: store.PortFixed, Port: 80}}, "servers.0.port"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: store.PortFixed, Port: 20013}}, "servers.0.port"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: "random"}}, "servers.0.port"},
		{"", []store.DevServer{{Name: "web", Command: "pnpm dev", Type: strings.Repeat("t", 33)}}, "servers.0.type"},
	} {
		r := store.Repo{Name: "shop", Path: dir, BaseBranch: "trunk", Setup: c.setup, Servers: c.servers}
		if _, err := s.Add(ctx, r); field(err) != c.field {
			t.Errorf("Add(%q, %+v): %v; want a problem with %s", c.setup, c.servers, err, c.field)
		}
	}

	added, err := s.Add(ctx, store.Repo{Name: "shop", Path: dir, BaseBranch: "trunk", Setup: " pnpm install ", Servers: []store.DevServer{
		{Name: " web ", Command: " pnpm dev --port $PORT ", Dir: "./apps/web/", Type: " rspack ", Port: 3000},
		{Name: "metro", Command: "pnpm mobile:start", Dir: ".", PortMode: store.PortFixed, Port: 8081},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []store.DevServer{
		{Name: "web", Command: "pnpm dev --port $PORT", Dir: "apps/web", PortMode: store.PortAssigned, Type: "rspack"},
		{Name: "metro", Command: "pnpm mobile:start", PortMode: store.PortFixed, Port: 8081},
	}
	if added.Setup != "pnpm install" || !slices.Equal(added.Servers, want) {
		t.Errorf("added setup %q, servers %+v; want them trimmed, cleaned and defaulted", added.Setup, added.Servers)
	}
}
