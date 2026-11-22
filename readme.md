# Stream Forge

Stream Forge is a Linux terminal application for Dispatcharr, Emby, and media administration. Run with no arguments on a TTY to open the interactive menu. Explicit commands run once and exit.

```sh
go build -o streamforge .
./streamforge dispatcharr --help
```

Set `DISP_URL`, `DISP_USER` and `DISP_PASS` in the process environment or in a read-only `/config/.env` file. Environment values take precedence. The CLI writes a timestamped session log under `/working`, including for dry runs.

| Command | Operation |
| --- | --- |
| `dispatcharr export` | Write all channels and TVG IDs to `/working/channels.csv`. |
| `dispatcharr refresh` | Request asynchronous M3U source refresh. |
| `dispatcharr create-update` | Synchronize channels with streams grouped by normalized name. |
| `dispatcharr prune` | Remove stale source streams and channels and groups emptied by the removal. |
| `dispatcharr all` | Request refresh, then run create/update, prune, match, renumber, and reorder. |
| `dispatcharr match` | Synchronize channel TVG IDs with their mapped EPG records. |
| `dispatcharr clear-epg` | Clear channel EPG mappings. |
| `dispatcharr renumber` | Number nonempty channels in alphabetical order. |
| `dispatcharr reorder` | Sort each channel's streams by M3U account name. |

Use `--dry-run` to preview an action and `--yes` to authorize changes. Use `--verbose` for debug logs. Export overwrites its CSV automatically.

The standalone refresh queues the job. `all` waits for active accounts that expose refresh status before continuing. A dry run previews each step against the current data.

| Optional environment variable | Default |
| --- | --- |
| `STREAMFORGE_TIMEOUT` | `30s` |
| `STREAMFORGE_START_CHANNEL` | `1` |
| `STREAMFORGE_NORMALIZER` | Empty. Optionally use a Go regular expression to strip parts of stream names. |
| `STREAMFORGE_VERIFY_TLS` | `true` |
| `STREAMFORGE_CONFIRM` | `true` |
| `STREAMFORGE_USER_AGENT` | `VLC/3.0.20 LibVLC/3.0.20` |

## Emby

Set `EMBY_HOST` to the Emby server URL and `EMBY_API_KEY` to its API key. For `logos` and `all`, also set `DISP_M3U_URL` to the Dispatcharr playlist URL.

| Command | Operation |
| --- | --- |
| `emby refresh` | Start guide refresh and wait for the result, polling every five seconds for up to 30 minutes. |
| `emby logos` | Update logo variants for uniquely matched channels. |
| `emby numbers` | Clear channel numbers and their associated field locks. |
| `emby all` | Refresh the guide, then update logos, then clear numbers. A failed refresh is reported and the channel updates still run. |

Use `--dry-run` to preview or `--yes` to apply changes.

## Xtream series discovery

Copy `providers.json.example` to the host config directory as `providers.json` and fill in the provider details. Search runs against all valid providers by default and lists exact title matches first within each provider. Use `--provider NAME` to filter the search.

```sh
streamforge series search "Example Series"
streamforge series episodes --provider "Example Provider" 123
```

Provider priority sorts from low to high. Provider headers can override the global user agent. The configuration also accepts download settings `concurrent`, `retries`, `retry_delay` as a duration, and aggregate `bandwidth_kbps` in KB/s, with 0 for unlimited.

## Downloads and media

Create `working/input` and `working/output`. Set `STREAMFORGE_PLAYBACK_PARENT` to the absolute player-visible directory that contains the mapped `input` and `output` trees. For example, `/library` yields playback paths such as `/library/input/show/episode.mkv`. Downloads are stored under a provider-specific directory and series ID in `input`; processed files and companions use the corresponding relative directory under `output`.

```sh
streamforge series download --provider "Example Provider" 123 --episode=456
streamforge series download --provider "Example Provider" 123 --all-episodes --process
streamforge series download --select="Example Provider|123|all" --select="Other Provider|987|321,322"
streamforge movies transcode movies --codec=hevc --container=mkv
streamforge series transcode shows --convert-hdr
streamforge strm input/shows --playback-parent=/library
streamforge diagnostics --json
streamforge diagnostics --save
```

`--redownload --yes` replaces an episode after FFprobe validation. `--force` permits reprocessing corresponding outputs while preserving existing media until a replacement succeeds. `--delete-original --yes` removes an original only after successful conversion. Use `--exclude=GLOBS` for comma-separated basename patterns. Set `TMDB_API_KEY` to enable companion filename enrichment. Media filenames remain as supplied.

Media commands use host-mounted FFmpeg and FFprobe. The default paths are `/usr/local/bin/ffmpeg` and `/usr/local/bin/ffprobe`. Use statically linked Linux binaries for the image architecture, or provide the runtime libraries and loader needed by the mounted executables. For GPU encoding, pass the device nodes, matching host libraries, and supplementary groups that grant read/write device access. Device paths and drivers depend on the host. NVIDIA container integration can supply its driver libraries and devices. Intel and AMD generally require the applicable `/dev/dri/renderD*` node and user-space driver. A usable CPU encoder allows fallback.

The default CPU budget is half the available logical CPUs. If GPU encoding is available, the default is one concurrent GPU job; an explicit job count is capped at two as a conservative limit. Set `--resolution=WIDTHxHEIGHT`, `--frame-rate=FPS`, or `--aspect=W:H` to override source geometry or timing. Stream copy carries the original HDR stream. HEVC 10-bit encoding supports static HDR metadata, and `--convert-hdr` uses the CPU tone-mapping filters. Dynamic HDR re-encoding depends on encoder support; use stream copy to retain the original stream.

## Interactive and scheduled use

Use `docker compose run --rm streamforge` to open the terminal interface. Choose Continue with Defaults or enter session settings. Arrow keys move, typing filters the current list, Enter selects, and Esc returns or cancels an active operation. Ctrl+C exits. Completed operations retain their results until Enter returns to the menu. For SSH, allocate a terminal with `ssh -t` when opening the TUI.

For series, search providers, press B to browse results, choose a series, and press B to select its episodes. Selections from different providers accumulate in the Download queue. During a download, X cancels the selected active transfer, and Esc cancels the batch.

For cron, pass the command explicitly and omit interactive options. For example, with the paths mounted by the supplied Compose file:

```cron
15 3 * * * cd /srv/streamforge && docker compose run --rm -T streamforge dispatcharr export
```

## Container

Mount an existing config directory and a writable working directory. The image runs with UID and GID 1000 by default. Use the `PUID` and `PGID` build arguments or the runtime's `--user` option to select an identity with access to those mounts.

```sh
docker run --rm --read-only \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" \
  ghcr.io/kpirnie/streamforge:dev dispatcharr export

podman run --rm --read-only --userns=keep-id \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" \
  ghcr.io/kpirnie/streamforge:dev dispatcharr export
```

Pushes to `develop` publish the `dev` tag, and pushes to `main` publish `latest`. Both tags target Linux amd64 and arm64.
