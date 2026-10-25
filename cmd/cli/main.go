// The cli command launches the game TUI, loading the embedded assets or a
// mod directory overlaid on them.
package main

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lckrugel/tower-of-doom/internal/game"
	"github.com/lckrugel/tower-of-doom/internal/game/engine"
	"github.com/lckrugel/tower-of-doom/internal/tui"
)

// version is the game version. Release builds override it at link time
// with: -ldflags "-X main.version=<version>".
var version = "dev"

// validateHorizon validates content coverage up to this floor.
// The tower is infinite (sections with max_floor == -1 cover the rest),
// so a large horizon is enough to guarantee valid content for everything
// reachable in a game.
const validateHorizon = 100_000

// assetFiles are the flat overlay files a mod directory may provide. A
// directory holding none of them is not a mod — notably an empty
// conventional mods directory, which must not flip provenance.
var assetFiles = []string{"monsters.json", "items.json", "moves.json", "statuses.json", "sections.json"}

const usage = `usage: tower-of-doom [flags]

  --mods <dir>   flat directory of mod asset JSONs to overlay on the
                 embedded assets (monsters, items, moves, statuses,
                 sections). Files missing from the directory fall back
                 to the embedded ones.
  --paths        print the resolved mods directory and where each asset
                 file came from, then exit without starting the game.
  --version      print the version and exit.

The mods directory resolves from --mods, then the TOD_MODS environment
variable, then the conventional per-user directory for this platform.
An empty or unusable env/default directory is ignored; an explicit
--mods directory must exist and hold at least one asset file.

exit codes: 0 ok · 1 invalid content · 2 usage error`

func main() {
	mods := flag.String("mods", "", "flat overlay directory of asset JSONs")
	paths := flag.Bool("paths", false, "print the resolved paths and asset provenance, then exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	flag.Parse()

	// flag.Visit distinguishes an explicit (even empty) --mods from an
	// absent one: only an explicit flag is a usage error when unusable.
	modsSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "mods" {
			modsSet = true
		}
	})

	if flag.NArg() > 0 {
		usageError(fmt.Sprintf("unexpected argument %q", flag.Arg(0)))
	}
	if *showVersion {
		fmt.Printf("tower-of-doom %s\n", version)
		return
	}
	if modsSet && *mods == "" {
		usageError("--mods needs a directory")
	}

	// Precedence: flag > $TOD_MODS > the platform convention.
	dir := resolveModsDir(*mods, os.Getenv)
	if modsSet {
		if err := checkModsDir(dir); err != nil {
			usageError(err.Error())
		}
	}

	// --paths is a diagnostic: it still loads and validates the content, so
	// a broken mod reports its errors instead of a false clean report, but
	// it never starts the TUI and never creates a directory.
	if *paths {
		registry, err := loadContent(modsLoadDir(dir))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printPaths(dir, registry)
		return
	}

	// Starting for real: create the conventional directory so the player
	// has somewhere to drop mod files. Never fatal — a read-only home must
	// not stop the game; the warning says we carry on with embedded assets.
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			fmt.Fprintf(os.Stderr, "warning: cannot create mods directory %s: %v; using embedded assets\n", dir, err)
		}
	}

	registry, err := loadContent(modsLoadDir(dir))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gameEngine := engine.NewEngine(registry)

	// WithAltScreen keeps the frame off the terminal scrollback: the game
	// screen is redrawn in place, and quitting restores the shell as it
	// was.
	if _, err := tea.NewProgram(tui.New(gameEngine), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error running the TUI:", err)
		os.Exit(1)
	}
}

// usageError prints a command-line misuse message plus the usage block and
// exits with the misuse code, mirroring cmd/validate.
func usageError(message string) {
	fmt.Fprintln(os.Stderr, message)
	fmt.Fprintln(os.Stderr, usage)
	os.Exit(2)
}

// resolveModsDir picks the mods directory: the --mods flag, then the
// TOD_MODS environment variable, then the platform's conventional per-user
// directory. It returns "" when nothing resolves.
func resolveModsDir(flagDir string, getenv func(string) string) string {
	if flagDir != "" {
		return flagDir
	}
	if envDir := getenv(game.ModsEnvVar); envDir != "" {
		return envDir
	}
	return game.DefaultModsDir(runtime.GOOS, getenv)
}

// modsLoadDir returns the directory to hand to game.Load: the mods
// directory ONLY when it actually holds at least one asset file. An empty
// or nonexistent default/env directory is ignored — it is not a mod and
// must not flip provenance away from "embedded".
func modsLoadDir(dir string) string {
	if dir != "" && hasAssetFiles(dir) {
		return dir
	}
	return ""
}

// hasAssetFiles reports whether dir holds at least one of the five flat
// overlay asset files.
func hasAssetFiles(dir string) bool {
	for _, name := range assetFiles {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// checkModsDir enforces the explicit --mods contract: the path must exist,
// be a directory and hold at least one asset file. Anything else is a
// usage error (exit 2) — the user named a source that cannot be a mod.
func checkModsDir(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return fmt.Errorf("mods directory %s: %v", dir, err)
	case !info.IsDir():
		return fmt.Errorf("mods path %s is not a directory", dir)
	case !hasAssetFiles(dir):
		return fmt.Errorf("mods directory %s holds no asset files (%s)",
			dir, strings.Join(assetFiles, ", "))
	}
	return nil
}

// loadContent loads the assets (from dir when non-empty, falling back per
// file to the embedded ones) and validates them. Both failures are fatal:
// bad content is a mod bug the player cannot play through.
func loadContent(dir string) (*game.Registry, error) {
	registry, err := game.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load assets: %w", err)
	}
	if err := registry.Validate(validateHorizon); err != nil {
		return nil, fmt.Errorf("invalid game content: %w", err)
	}
	return registry, nil
}

// printPaths is the --paths report: the version, where mods resolve to,
// whether that directory exists, and where each asset file came from —
// "assets: embedded" when everything is compiled in, otherwise one
// "<name>: <source>" line per file, sorted by file name.
func printPaths(dir string, registry *game.Registry) {
	fmt.Printf("tower-of-doom %s\n", version)

	if dir == "" {
		fmt.Println("mods dir: (none)")
	} else {
		fmt.Printf("mods dir: %s (%s)\n", dir, dirState(dir))
	}

	provenance := registry.Provenance()
	names := slices.Sorted(maps.Keys(provenance))

	allEmbedded := true
	for _, name := range names {
		if provenance[name] != "embedded" {
			allEmbedded = false
			break
		}
	}
	if allEmbedded {
		fmt.Println("assets: embedded")
		return
	}
	for _, name := range names {
		fmt.Printf("%s: %s\n", strings.TrimSuffix(name, ".json"), provenance[name])
	}
}

// dirState describes a resolved mods directory for the --paths report.
func dirState(dir string) string {
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return "missing"
	case !info.IsDir():
		return "not a directory"
	default:
		return "exists"
	}
}
