// The validate command checks game assets — the embedded defaults or a
// mod directory — and prints a digest when they are valid, or every
// content fault when they are not, so modders can check their content
// without launching the game.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lckrugel/tower-of-doom/internal/game"
)

// version is the validator version. Release builds override it at link
// time with: -ldflags "-X main.version=<version>".
var version = "dev"

// validateHorizon bounds floor-budget validation. An unbounded tail section
// describes coverage beyond this point, but this finite walk does not verify
// every future floor's difficulty budget.
const validateHorizon = 100_000

const usage = `usage: validate [--version] [path]

  path       optional directory of game assets to validate (mod directory).
             Without it, the embedded default assets are validated.
             Files missing from the directory fall back to the embedded ones.
  --version  print the version and exit.

Unreadable or broken asset files are invalid CONTENT (exit 1) — they
are what the modder must fix. Exit 2 is reserved for command-line
misuse.

exit codes: 0 valid · 1 invalid content · 2 usage error`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stdout))
}

// run validates command-line input and writes diagnostics without exiting the
// process, making argument and exit-code behavior directly testable.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		fmt.Fprintf(stdout, "tower-validate %s\n", version)
		return 0
	}
	// At most one positional path; any flag or extra argument is misuse.
	if len(args) > 1 || (len(args) == 1 && strings.HasPrefix(args[0], "-")) {
		fmt.Fprintln(stderr, usage)
		return 2
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
			return fail(stdout, fmt.Errorf("reading asset directory %s: %w", dir, err))
		} else if !info.IsDir() {
			return fail(stdout, fmt.Errorf("asset path %s is not a directory", dir))
		}
	}

	// Load failures (unreadable/broken external files, duplicate ids,
	// bad JSON) are content bugs the modder must fix: exit 1, not 2.
	registry, err := game.Load(dir)
	if err != nil {
		return fail(stdout, err)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		return fail(stdout, err)
	}

	printDigest(stdout, registry)
	return 0
}

// fail prints the error report — every fault on its own line — and
// exits with the invalid-content code. The report goes to stdout: it
// is the command's product (pipe it, redirect it), not a malfunction.
func fail(out io.Writer, err error) int {
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	fmt.Fprintf(out, "FAILED (%d errors)\n", len(lines))
	return 1
}

// printDigest summarizes valid content: where each asset file came from,
// how much of it there is, the finite floor-validation horizon and the tier
// ladders. Plain lines, one fact each, ending in OK.
func printDigest(out io.Writer, r *game.Registry) {
	provenance := r.Provenance()
	for _, name := range []string{"monsters", "items", "moves", "statuses", "sections"} {
		fmt.Fprintf(out, "%s: %s\n", name, provenance[name+".json"])
	}
	fmt.Fprintf(out, "monsters: %d\n", len(r.Monsters))
	fmt.Fprintf(out, "items: %d\n", len(r.Items))
	fmt.Fprintf(out, "moves: %d\n", len(r.Moves))
	fmt.Fprintf(out, "statuses: %d\n", len(r.Statuses))
	fmt.Fprintf(out, "sections: %d\n", len(r.Sections))

	fmt.Fprintf(out, "validated floors: 1-%d (finite validation horizon)\n", validateHorizon)

	fmt.Fprintf(out, "item tiers: %s\n", tierSpan(r.Items, func(i game.ItemAsset) int { return i.Tier }))
	fmt.Fprintf(out, "move tiers: %s\n", tierSpan(r.Moves, func(m game.MoveAsset) int { return m.Tier }))
	fmt.Fprintln(out, "OK")
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
