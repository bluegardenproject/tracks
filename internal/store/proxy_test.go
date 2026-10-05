package store

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestProxyPorts(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	for _, port := range []int{3000, 8081} {
		if err := s.AddProxyPort(ctx, port); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddProxyPort(ctx, 3000); !errors.Is(err, ErrPortTaken) {
		t.Errorf("adding 3000 twice: %v; want ErrPortTaken", err)
	}
	web := ProxyInput{Track: "t1", Repo: "api", Server: "web"}
	if err := s.SetProxyInput(ctx, 3000, web); err != nil {
		t.Fatal(err)
	}
	got, err := s.ProxyPorts(ctx)
	want := []ProxyPort{{3000, web}, {8081, ProxyInput{}}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("ProxyPorts = %+v, %v; want %+v", got, err, want)
	}
	if err := s.RemoveProxyPort(ctx, 3000); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProxyPort(ctx, 3000); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing 3000 twice: %v; want ErrNotFound", err)
	}
	if err := s.SetProxyInput(ctx, 3000, web); !errors.Is(err, ErrNotFound) {
		t.Errorf("setting a removed port's input: %v; want ErrNotFound", err)
	}
	if got, _ := s.ProxyPorts(ctx); len(got) != 1 || got[0].Port != 8081 || !got[0].Input.None() {
		t.Errorf("after removing 3000: %+v", got)
	}
}
