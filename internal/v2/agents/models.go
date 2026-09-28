package agents

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// The model list is a network call, and this CLI can hang.
const listTimeout = 10 * time.Second

// ListModels asks the CLI at path which models the account can use.
func ListModels(ctx context.Context, path string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--list-models")
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, errors.New("the model list took too long")
	}
	if err != nil {
		return nil, fmt.Errorf("%s --list-models: %w", path, err)
	}
	models := parseModelList(string(out))
	if len(models) == 0 {
		return nil, errors.New("the model list was empty; is the CLI logged in?")
	}
	return models, nil
}

// parseModelList reads the `id - Label` lines `agent --list-models`
// prints, ignoring its header and any warning noise around them. "auto"
// is left out: it's the CLI's own default, which passing no --model
// already chooses.
func parseModelList(out string) []Model {
	var models []Model
	for _, line := range strings.Split(out, "\n") {
		id, label, ok := strings.Cut(strings.TrimSpace(line), " - ")
		if !ok {
			continue
		}
		id, label = strings.TrimSpace(clean(id)), strings.TrimSpace(clean(label))
		// An id is a single token; anything with a space in it is prose
		// that happened to contain " - ".
		if id == "" || label == "" || strings.ContainsAny(id, " \t") || id == "auto" {
			continue
		}
		models = append(models, Model{ID: id, Label: label})
	}
	return models
}
