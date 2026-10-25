package game

import "path/filepath"

// ModsEnvVar is the environment variable that overrides the conventional
// per-user mods directory. The CLI reads it (like any mods source, an
// empty or unusable value is ignored); DefaultModsDir itself never looks
// at the process environment — the caller injects it.
const ModsEnvVar = "TOD_MODS"

// DefaultModsDir returns the conventional per-user directory where mods
// (flat overlay asset files) live, or "" when the platform gives no home
// to resolve against.
//
// It is a pure convention table: `goos` selects which platform's layout
// applies, and `getenv` (typically os.Getenv) supplies the environment.
// No globals are read, nothing is created and no filesystem is touched —
// the caller decides what to do with the answer.
//
//   - linux and the other unices: $XDG_DATA_HOME/tower-of-doom/mods when
//     XDG_DATA_HOME is a non-empty ABSOLUTE path (the XDG spec requires
//     absolute paths, so a relative one is ignored), otherwise
//     $HOME/.local/share/tower-of-doom/mods.
//   - darwin: $HOME/Library/Application Support/tower-of-doom/mods.
//   - windows: %APPDATA%\tower-of-doom\mods, falling back to
//     $HOME/AppData/Roaming/tower-of-doom/mods when APPDATA is empty.
//
// `goos` only selects the CONVENTION: paths are joined with
// filepath.Join, which follows the HOST's separator semantics, not the
// named platform's. The result is a host path (for tests and for the
// running machine's own layout), never a synthetic foreign path.
func DefaultModsDir(goos string, getenv func(string) string) string {
	home := getenv("HOME")

	switch goos {
	case "windows":
		if appdata := getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "tower-of-doom", "mods")
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, "AppData", "Roaming", "tower-of-doom", "mods")

	case "darwin":
		if home == "" {
			return ""
		}
		return filepath.Join(home, "Library", "Application Support", "tower-of-doom", "mods")

	default: // linux and the other unices
		if xdg := getenv("XDG_DATA_HOME"); xdg != "" && filepath.IsAbs(xdg) {
			return filepath.Join(xdg, "tower-of-doom", "mods")
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, ".local", "share", "tower-of-doom", "mods")
	}
}
