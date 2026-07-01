package services

import (
	"strings"
	"testing"
)

func TestStartOrderDependenciesFirst(t *testing.T) {
	// live-app depends on lld, so lld must come first.
	names := []string{"live-app", "lld"}
	deps := map[string][]string{"live-app": {"lld"}}
	order, err := StartOrder(names, deps)
	if err != nil {
		t.Fatalf("StartOrder: %v", err)
	}
	if pos(order, "lld") > pos(order, "live-app") {
		t.Errorf("lld should precede live-app, got %v", order)
	}
	if len(order) != 2 {
		t.Errorf("expected both services, got %v", order)
	}
}

func TestStartOrderIgnoresDepsOutsideSet(t *testing.T) {
	// A dependency on a service not being started is skipped, not an error
	// (config validation already guarantees it exists).
	order, err := StartOrder([]string{"a"}, map[string][]string{"a": {"ghost"}})
	if err != nil {
		t.Fatalf("StartOrder: %v", err)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Errorf("expected [a], got %v", order)
	}
}

func TestStartOrderChain(t *testing.T) {
	// c → b → a: a before b before c regardless of input order.
	deps := map[string][]string{"c": {"b"}, "b": {"a"}}
	order, err := StartOrder([]string{"c", "b", "a"}, deps)
	if err != nil {
		t.Fatalf("StartOrder: %v", err)
	}
	if pos(order, "a") > pos(order, "b") || pos(order, "b") > pos(order, "c") {
		t.Errorf("expected a,b,c order, got %v", order)
	}
}

func TestStartOrderCycleErrors(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	if _, err := StartOrder([]string{"a", "b"}, deps); err == nil {
		t.Fatal("expected cycle error, got nil")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

func pos(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
