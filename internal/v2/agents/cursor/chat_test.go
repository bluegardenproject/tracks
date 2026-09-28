package cursor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeAgent(t *testing.T, script string) string {
	path := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateChat(t *testing.T) {
	id := "0f8fad5b-d9cb-469f-a165-70867728950e"
	agent := fakeAgent(t, `echo "WARNING: trust settings"; echo `+id)
	got, err := CreateChat(context.Background(), agent)
	if err != nil || got != id {
		t.Fatalf("CreateChat = %q, %v; want %q", got, err, id)
	}

	agent = fakeAgent(t, `echo "not logged in" >&2; exit 1`)
	if _, err := CreateChat(context.Background(), agent); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("failure: %v", err)
	}

	agent = fakeAgent(t, `echo "a line that is 36 characters long.."`)
	if _, err := CreateChat(context.Background(), agent); err == nil {
		t.Error("output without an ID was taken as a chat")
	}
}
