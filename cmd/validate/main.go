// The validate command checks game assets — the embedded defaults or a
// mod directory — and prints a digest when they are valid, or every
// content fault when they are not, so modders can check their content
// without launching the game.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// validateHorizon validates content coverage up to this floor.
// Mirrors cmd/cli: the tower is infinite (a tail section with
// max_floor == -1 covers the rest), so a large horizon is enough to
// guarantee valid content for everything reachable in a game.
const validateHorizon = 100_000

const usage = `usage: validate [path]

  path   optional directory of game assets to validate (mod directory).
         Without it, the embedded default assets are validated.
         Files missing from the directory fall back to the embedded ones.

Unreadable or broken asset files are invalid CONTENT (exit 1) — they
are what the modder must fix. Exit 2 is reserved for command-line
misuse.

exit codes: 0 valid · 1 invalid content · 2 usage error`

func main() {
	args := os.Args[1:]
	// At most one positional path; any flag or extra argument is misuse.
	if len(args) > 1 || (len(args) == 1 && strings.HasPrefix(args[0], "-")) {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	dir := ""
	if len(args) == 1 {
		dir = args[0]
	}

	// A path that cannot be read at all (typo, permissions, not a
	// directory) is a content failure the modder must fix — a clear
	// error, exit 1 — NOT a silent fallback to the embedded assets.
	if dir != "" {
		if info, err := os.Stat(dir); err != nil {
			fail(fmt.Errorf("reading asset directory %s: %w", dir, err))
		} else if !info.IsDir() {
			fail(fmt.Errorf("asset path %s is not a directory", dir))
		}
	}

	// Load failures (unreadable/broken external files, duplicate ids,
	// bad JSON) are content bugs the modder must fix: exit 1, not 2.
	registry, err := game.Load(dir)
	if err != nil {
		fail(err)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		fail(err)
	}

	printDigest(registry)
}

// fail prints the error report — every fault on its own line — and
// exits with the invalid-content code. The report goes to stdout: it
// is the command's product (pipe it, redirect it), not a malfunction.
func fail(err error) {
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	for _, line := range lines {
		fmt.Println(line)
	}
	fmt.Printf("FAILED (%d errors)\n", len(lines))
	os.Exit(1)
}

// printDigest summarizes valid content: where each asset file came
// from, how much of it there is, the covered floor span and the tier
// ladders. Plain lines, one fact each, ending in OK.
func printDigest(r *game.Registry) {
	provenance := r.Provenance()
	for _, name := range []string{"monsters", "items", "moves", "statuses", "sections"} {
		fmt.Printf("%s: %s\n", name, provenance[name+".json"])
	}
	fmt.Printf("monsters: %d\n", len(r.Monsters))
	fmt.Printf("items: %d\n", len(r.Items))
	fmt.Printf("moves: %d\n", len(r.Moves))
	fmt.Printf("statuses: %d\n", len(r.Statuses))
	fmt.Printf("sections: %d\n", len(r.Sections))

	// The validated span starts at floor 1 and reaches the horizon — or
	// forever, when a tail section (max_floor == -1) has no end.
	last := fmt.Sprint(validateHorizon)
	for _, s := range r.Sections {
		if s.MaxFloor == -1 {
			last = "∞"
			break
		}
	}
	fmt.Printf("floors: 1-%s\n", last)

	fmt.Printf("item tiers: %s\n", tierSpan(r.Items, func(i game.ItemAsset) int { return i.Tier }))
	fmt.Printf("move tiers: %s\n", tierSpan(r.Moves, func(m game.MoveAsset) int { return m.Tier }))
	fmt.Println("OK")
}

// tierSpan formats the tier ladder present among the assets as
// "min-max", or "none" when there are none. Validation guarantees every
// tier is >= 1, so a zero max means no assets at all.
func tierSpan[T any](assets map[string]T, tier func(T) int) string {
	lo, hi := 0, 0
	for _, asset := range assets {
		t := tier(asset)
		if lo == 0 || t < lo {
			lo = t
		}
		if t > hi {
			hi = t
		}
	}
	if hi == 0 {
		return "none"
	}
	return fmt.Sprintf("%d-%d", lo, hi)
}
