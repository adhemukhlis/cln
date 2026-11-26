// Package clean permanently deletes the cleanup candidates found by a scan.
package clean

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/adhemukhlis/cln/internal/ignore"
	"github.com/adhemukhlis/cln/internal/scan"
)

// Item is one path to delete.
type Item struct {
	Category string // Result key: "node_modules", "misc" or "framework".
	Display  string // "~/" form, as written in result.json.
	Path     string // Absolute path inside home.
}

// Plan is the validated list of items to delete.
type Plan struct {
	Items []Item
	// Ignored counts result entries dropped because ignore.json covers them.
	Ignored int
}

// Count returns the number of items in category.
func (p Plan) Count(category string) int {
	n := 0
	for _, it := range p.Items {
		if it.Category == category {
			n++
		}
	}
	return n
}

// NewPlan validates every entry of res against home and drops the ones that
// ignored covers. Any entry outside home (or home itself) fails the whole
// plan, so a hand-edited or stale result.json can never delete outside home.
func NewPlan(home string, res scan.Result, ignored []string) (Plan, error) {
	var plan Plan
	categories := []struct {
		name    string
		entries []string
	}{
		{name: "node_modules", entries: res.NodeModules},
		{name: "misc", entries: res.Misc},
		{name: "framework", entries: res.Framework},
	}
	for _, c := range categories {
		for _, entry := range c.entries {
			path, err := scan.Expand(home, entry)
			if err != nil {
				return Plan{}, fmt.Errorf("%s: %s: %w", scan.ResultFileName, c.name, err)
			}
			if ignore.Covers(ignored, path) {
				plan.Ignored++
				continue
			}
			plan.Items = append(plan.Items, Item{Category: c.name, Display: entry, Path: path})
		}
	}
	return plan, nil
}

// Failure is an item that could not be deleted.
type Failure struct {
	Item Item
	Err  error
}

// Report summarizes an Execute run.
type Report struct {
	Deleted int
	Missing int // Already gone; not an error.
	Failed  []Failure
}

// Execute permanently deletes every item in order. A failed item is recorded
// and does not stop the others. It stops early, returning the partial report
// and an error, only when ctx is canceled. When onItem is not nil it is called
// before each item with the number of items already processed.
func Execute(ctx context.Context, items []Item, onItem func(done int, it Item)) (Report, error) {
	return execute(ctx, items, onItem, os.RemoveAll)
}

func execute(ctx context.Context, items []Item, onItem func(int, Item), remove func(string) error) (Report, error) {
	var rep Report
	for i, it := range items {
		if onItem != nil {
			onItem(i, it)
		}
		if err := ctx.Err(); err != nil {
			return rep, fmt.Errorf("clean interrupted: %w", err)
		}
		// Lstat, not Stat: a symlink that replaced a candidate is removed
		// itself and never followed.
		if _, err := os.Lstat(it.Path); errors.Is(err, fs.ErrNotExist) {
			rep.Missing++
			continue
		}
		if err := remove(it.Path); err != nil {
			rep.Failed = append(rep.Failed, Failure{Item: it, Err: err})
			continue
		}
		rep.Deleted++
	}
	return rep, nil
}
