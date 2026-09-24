package demo

import (
	"context"
	"fmt"
	"io"
	"time"
)

var requests = []string{
	"GET /health 200 1ms",
	"GET /api/cart 200 14ms",
	"POST /api/cart/items 201 32ms",
	"GET /api/cart 200 11ms",
	"GET /static/app.js 304 2ms",
}

// RunDevServer prints a fake dev-server log to w until ctx ends.
func RunDevServer(ctx context.Context, w io.Writer, name string, port int) error {
	fmt.Fprintf(w, "%s listening on http://localhost:%d\n", name, port)
	for i := 0; ; i++ {
		if sleep(ctx, 3*time.Second) != nil {
			return nil
		}
		fmt.Fprintf(w, "%s  %s\n", time.Now().Format("15:04:05"), requests[i%len(requests)])
	}
}
