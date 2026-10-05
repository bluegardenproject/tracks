package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// TestProxyPorts adds an output port through the daemon: without an
// input it isn't listened on; with an input that isn't running it
// answers 503; removed, it closes.
func TestProxyPorts(t *testing.T) {
	c, _ := config(t)
	done := start(t, c)
	client := rpc.Client{Socket: c.Paths.Socket}
	ctx := context.Background()
	defer func() {
		_ = client.Shutdown(ctx)
		_ = wait(t, done)
	}()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	view, err := client.AddProxyPort(ctx, port)
	if err != nil || len(view.Ports) != 1 || view.Ports[0].State != tracks.ProxyIdle {
		t.Fatalf("AddProxyPort = %+v, %v; want it idle", view, err)
	}
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port)); err == nil {
		t.Error("an idle port is listened on")
	}
	if _, err := client.AddProxyPort(ctx, 80); err == nil {
		t.Error("port 80 was added")
	}

	view, err = client.SetProxyInput(ctx, port, store.ProxyInput{Track: "gone", Repo: "api", Server: "web"})
	if err != nil || view.Ports[0].State != tracks.ProxyNoServer {
		t.Fatalf("SetProxyInput = %+v, %v; want no server", view, err)
	}
	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil || res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("a port whose input isn't running: %v, %v; want 503", res, err)
	}
	res.Body.Close()
	if cached, err := client.Proxy(ctx, false); err != nil || len(cached.Ports) != 1 || cached.Ports[0].State != tracks.ProxyNoServer {
		t.Errorf("the cached view: %+v, %v; want the last sync's port", cached, err)
	}

	if view, err = client.RemoveProxyPort(ctx, port); err != nil || len(view.Ports) != 0 {
		t.Fatalf("RemoveProxyPort = %+v, %v", view, err)
	}
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port)); err == nil {
		t.Error("a removed port still answers")
	}
}
