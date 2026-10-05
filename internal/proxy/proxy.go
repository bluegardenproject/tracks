// Package proxy forwards Tracks' output ports to dev servers: each
// output port listens on the loopback addresses only, and forwards HTTP
// and WebSockets to the upstream port it's set to.
package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Proxy is the output ports being listened on.
type Proxy struct {
	mu    sync.Mutex
	ports map[int]*output
}

// output is one output port: its listeners and what it forwards to,
// nil for nothing.
type output struct {
	servers  []*http.Server
	upstream atomic.Pointer[upstream]
}

// upstream is a port forwarded to.
type upstream struct {
	port int
	rp   *httputil.ReverseProxy
}

// Set makes the proxy listen on want's ports, each forwarding to its
// upstream port, or answering 503 for 0. Ports not in want are closed.
// It returns why ports couldn't be listened on, such as another program
// holding them; Set tries them again next time.
func (p *Proxy) Set(want map[int]int) map[int]error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ports == nil {
		p.ports = map[int]*output{}
	}
	for port, o := range p.ports {
		if _, ok := want[port]; !ok {
			o.close()
			delete(p.ports, port)
		}
	}
	errs := map[int]error{}
	for port, upstream := range want {
		o, ok := p.ports[port]
		if !ok {
			var err error
			if o, err = listen(port); err != nil {
				errs[port] = err
				continue
			}
			p.ports[port] = o
		}
		o.point(upstream)
	}
	return errs
}

// point forwards o to port, or to nothing for 0. Connections already
// upgraded, such as an HMR WebSocket, stay with the old upstream until
// they close; the dev server's client reconnects.
func (o *output) point(port int) {
	if cur := o.upstream.Load(); (cur == nil && port == 0) || (cur != nil && cur.port == port) {
		return
	}
	if port == 0 {
		o.upstream.Store(nil)
		return
	}
	o.upstream.Store(&upstream{port: port, rp: reverse(port)})
}

// Close stops listening on every port. Upgraded connections, which
// http.Server.Close leaves, close with their ends.
func (p *Proxy) Close() { p.Set(nil) }

// listen opens port on 127.0.0.1 and ::1. A machine without ::1 gets
// 127.0.0.1 alone; any other failure closes what opened.
func listen(port int) (*output, error) {
	o := &output{}
	handler := http.HandlerFunc(o.serve)
	for _, host := range []string{"127.0.0.1", "::1"} {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			if host == "::1" && !errors.Is(err, syscall.EADDRINUSE) {
				continue
			}
			o.close()
			return nil, err
		}
		srv := &http.Server{Handler: handler, ReadHeaderTimeout: 30 * time.Second}
		o.servers = append(o.servers, srv)
		go func() { _ = srv.Serve(ln) }()
	}
	return o, nil
}

func (o *output) close() {
	for _, srv := range o.servers {
		_ = srv.Close()
	}
}

// serve forwards r upstream. The Host header stays the one the client
// sent, so a dev server builds its URLs, HMR included, for the output
// port.
func (o *output) serve(w http.ResponseWriter, r *http.Request) {
	up := o.upstream.Load()
	if up == nil {
		http.Error(w, "Tracks: no server is running behind this port.", http.StatusServiceUnavailable)
		return
	}
	up.rp.ServeHTTP(w, r)
}

// reverse forwards to localhost:upstream, which reaches a server on
// either loopback: Vite on macOS often listens on ::1 alone.
func reverse(upstream int) *httputil.ReverseProxy {
	target := &url.URL{Scheme: "http", Host: fmt.Sprintf("localhost:%d", upstream)}
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = r.In.Host
			r.SetXForwarded()
		},
		// Streams such as server-sent events go out as they come.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, fmt.Sprintf("Tracks: the server on port %d didn't answer: %v", upstream, err), http.StatusBadGateway)
		},
	}
}
