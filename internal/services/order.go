package services

import "fmt"

// StartOrder topologically orders names so every dependency precedes the
// services that depend on it. deps maps a service name to the names it
// depends on; dependencies outside the requested set are ignored (they're
// validated upstream). The graph is expected acyclic — config validation
// rejects cycles — but a cycle here still yields an error rather than
// looping forever.
func StartOrder(names []string, deps map[string][]string) ([]string, error) {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	inSet := make(map[string]bool, len(names))
	for _, n := range names {
		inSet[n] = true
	}
	mark := make(map[string]int, len(names))
	order := make([]string, 0, len(names))

	var visit func(string) error
	visit = func(n string) error {
		switch mark[n] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("depends_on cycle at %q", n)
		}
		mark[n] = visiting
		for _, d := range deps[n] {
			if !inSet[d] {
				continue
			}
			if err := visit(d); err != nil {
				return err
			}
		}
		mark[n] = done
		order = append(order, n)
		return nil
	}

	for _, n := range names {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}
