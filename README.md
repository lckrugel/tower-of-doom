# Tower of Doom

A roguelike for the terminal: one hero, a tower of escalating floors, and
whatever is waiting on them.

[![ci](https://github.com/lckrugel/tower-of-doom/actions/workflows/ci.yml/badge.svg)](https://github.com/lckrugel/tower-of-doom/actions/workflows/ci.yml)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/lckrugel/tower-of-doom/main/install.sh | sh
```

The script picks the archive for your OS and architecture, verifies its sha256
against the release's `checksums.txt`, and installs `tower-of-doom` and
`tower-validate` into `~/.local/bin`. It needs only curl (or wget), tar and
sha256sum (or shasum), and nothing reaches `BIN_DIR` until the download is
verified.

Choose another directory or pin a version with environment variables:

```sh
curl -fsSL https://raw.githubusercontent.com/lckrugel/tower-of-doom/main/install.sh | BIN_DIR=~/bin sh
curl -fsSL https://raw.githubusercontent.com/lckrugel/tower-of-doom/main/install.sh | VERSION=v0.1.0 sh
```

Alternatively download an archive from
[Releases](https://github.com/lckrugel/tower-of-doom/releases) and extract
`tower-of-doom` (and `tower-validate`) yourself: `.tar.gz` for linux and
macOS, `.zip` for Windows. The installer does not support Windows — use the
manual download there.

## Run

```sh
tower-of-doom                 # start the game
tower-of-doom --version       # print the version and exit
tower-of-doom --paths         # print the resolved mods directory and asset provenance
tower-of-doom --mods ~/my-mod # play with a specific mod directory
```

`--paths` still loads and validates the content, then exits without starting
the TUI. Exit codes are shared by the commands: `0` ok, `1` invalid content,
`2` usage error.

## Modding

A mod is a flat directory holding any of five JSON files:

`monsters.json` · `items.json` · `moves.json` · `statuses.json` · `sections.json`

The overlay is per file: a file present in the directory replaces the embedded
one as a whole, and any file you leave out falls back to the embedded version.
A directory holding at least one of the five counts as a mod.

The game looks in the conventional per-user directory unless told otherwise
(`--mods`, then `$TOD_MODS`, then the default):

| Platform | Default mods directory |
| --- | --- |
| Linux | `$XDG_DATA_HOME/tower-of-doom/mods`, else `~/.local/share/tower-of-doom/mods` |
| macOS | `~/Library/Application Support/tower-of-doom/mods` |
| Windows | `%APPDATA%\tower-of-doom\mods` |

The conventional directory is created on start, so you can drop files in and
they are picked up on the next run. `TOD_MODS` points at a directory anywhere.

Check the content before playing:

```sh
tower-validate            # validate the embedded assets
tower-validate ~/my-mod   # validate a mod directory
```

It prints where each asset file came from, the number of entries per table,
the covered floor span and the tier ladders, ending in `OK` — or every content
fault it found. Exit codes: `0` valid, `1` invalid content
(including an unreadable directory), `2` usage error. `docs/MECHANICS.md`
describes the content itself.

## Development

```sh
go run ./cmd/cli        # run the game from source
go run ./cmd/validate   # run the asset validator from source
go test ./...
```

## Releases

Releases are tag-driven; the tag must point at a commit that is green in CI.

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The tag triggers the release workflow, GoReleaser builds `tower-of-doom` and
`tower-validate` for linux, macOS and Windows on amd64 and arm64, and publishes
one archive per platform plus `checksums.txt`. The version is injected at link
time (`-X main.version`), so `--version` reports the release tag.
