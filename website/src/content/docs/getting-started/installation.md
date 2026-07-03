---
title: Installation
description: Install rune from a release binary, go install, or source.
---

## Release binaries

Download the archive for your platform from the
[releases page](https://github.com/mmcdole/rune/releases), unpack it, and put
`rune` somewhere on your `PATH`:

```sh
tar xzf rune_*_linux_amd64.tar.gz
sudo mv rune /usr/local/bin/
rune -version
```

Builds cover Linux, macOS, and Windows (amd64 and arm64). Windows archives
are zips containing `rune.exe`.

## With Go

```sh
go install github.com/mmcdole/rune/cmd/rune@latest
```

## From source

```sh
git clone https://github.com/mmcdole/rune
cd rune
go build ./cmd/rune/
```

## Where things live

| | |
|---|---|
| Config & scripts | `~/.config/rune/` (`%APPDATA%\rune` on Windows) |
| Your main script | `~/.config/rune/init.lua`, auto-loaded at startup |
| Bookmarks & durable state | `~/.config/rune/store.json` |
| Session logs | `~/.config/rune/logs/` |

Nothing is created until it's needed; a fresh install is just the binary.

## Next

[Your First Session](/getting-started/first-session/) covers
connecting, bookmarks, and the basics of moving around.
