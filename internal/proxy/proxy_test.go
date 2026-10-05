package proxy

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
)

// freePort is a port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func portOf(t *testing.T, s *httptest.Server) int {
	t.Helper()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(s.URL, "http://"))
	var n int
	fmt.Sscan(port, &n)
	return n
}

func get(t *testing.T, port int) (int, string) {
	t.Helper()
	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/x", port))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func TestForwardsAndSwitches(t *testing.T) {
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "a "+r.Host) }))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "b") }))
	defer b.Close()
	out := freePort(t)
	p := &Proxy{}
	defer p.Close()

	if errs := p.Set(map[int]int{out: portOf(t, a)}); len(errs) != 0 {
		t.Fatal(errs)
	}
	if code, body := get(t, out); code != 200 || body != fmt.Sprintf("a 127.0.0.1:%d", out) {
		t.Errorf("through the proxy: %d %q; want a, with the client's Host", code, body)
	}
	p.Set(map[int]int{out: portOf(t, b)})
	if _, body := get(t, out); body != "b" {
		t.Errorf("after switching: %q, want b", body)
	}
	p.Set(map[int]int{out: 0})
	if code, _ := get(t, out); code != http.StatusServiceUnavailable {
		t.Errorf("without a server: %d, want 503", code)
	}
	b.Close()
	p.Set(map[int]int{out: portOf(t, b)})
	if code, _ := get(t, out); code != http.StatusBadGateway {
		t.Errorf("with the server gone: %d, want 502", code)
	}
	p.Set(nil)
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", out)); err == nil {
		t.Error("a removed port still answers")
	}
}

func TestBlockedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	p := &Proxy{}
	defer p.Close()
	if errs := p.Set(map[int]int{port: 1}); !errors.Is(errs[port], syscall.EADDRINUSE) {
		t.Fatalf("a port another program holds: %v; want EADDRINUSE", errs)
	}
	ln.Close()
	if errs := p.Set(map[int]int{port: 1}); len(errs) != 0 {
		t.Errorf("once it's free: %v; want it taken", errs)
	}
}

// TestWebSocket checks an Upgrade goes through both ways, as HMR needs.
func TestWebSocket(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		rw.Flush()
		line, _ := rw.ReadString('\n')
		rw.WriteString("echo " + line)
		rw.Flush()
	}))
	defer up.Close()
	out := freePort(t)
	p := &Proxy{}
	defer p.Close()
	p.Set(map[int]int{out: portOf(t, up)})

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", out))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET /hmr HTTP/1.1\r\nHost: localhost:%d\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n", out)
	r := bufio.NewReader(conn)
	res, err := http.ReadResponse(r, nil)
	if err != nil || res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v, %v", res, err)
	}
	fmt.Fprint(conn, "ping\n")
	if line, _ := r.ReadString('\n'); line != "echo ping\n" {
		t.Errorf("after the upgrade got %q", line)
	}
}
