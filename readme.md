<h1><img src="docs/assets/streamforge-logo.png" alt="Stream Forge" width="640"></h1>

## Badges

[![Build Main](https://img.shields.io/github/actions/workflow/status/kpirnie/streamforge/build.yml?branch=main&label=Main&logoColor=white&logo=github&labelColor=000&style=for-the-badge)](https://github.com/kpirnie/streamforge/actions/workflows/build.yml?query=branch%3Amain)
[![Build Develop](https://img.shields.io/github/actions/workflow/status/kpirnie/streamforge/build.yml?branch=develop&logoColor=white&label=Develop&logo=github&labelColor=000&style=for-the-badge)](https://github.com/kpirnie/streamforge/actions/workflows/build.yml?query=branch%3Adevelop)
[![GitHub Issues](https://img.shields.io/github/issues/kpirnie/streamforge?style=for-the-badge&logo=github&color=006400&logoColor=white&labelColor=000)](https://github.com/kpirnie/streamforge/issues)
[![GitHub Security](https://img.shields.io/badge/Security-View-6495ED?link=https://github.com/kpirnie/streamforge/security&logoColor=white&style=for-the-badge&labelColor=000&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgd2lkdGg9IjI0IiBoZWlnaHQ9IjI0IiBmaWxsPSJub25lIiBzdHJva2U9IndoaXRlIiBzdHJva2Utd2lkdGg9IjIiPjxwYXRoIGQ9Ik0xMiAyTDQgNXY2LjA5YzAgNS4wNSAzLjQxIDkuNzYgOCAxMC45MSA0LjU5LTEuMTUgOC01Ljg2IDgtMTAuOTFWNWwtOC0zeiIvPjwvc3ZnPg==)](https://github.com/kpirnie/streamforge/security)
[![Last Commit](https://img.shields.io/github/last-commit/kpirnie/streamforge?style=for-the-badge&labelColor=000&logoColor=white&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSJ3aGl0ZSIgc3Ryb2tlLXdpZHRoPSIxLjgiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+PHJlY3QgeD0iMyIgeT0iNC41IiB3aWR0aD0iMTgiIGhlaWdodD0iMTYuNSIgcng9IjIiLz48bGluZSB4MT0iMyIgeTE9IjkuNSIgeDI9IjIxIiB5Mj0iOS41Ii8+PGxpbmUgeDE9IjgiIHkxPSIyLjUiIHgyPSI4IiB5Mj0iNi41Ii8+PGxpbmUgeDE9IjE2IiB5MT0iMi41IiB4Mj0iMTYiIHkyPSI2LjUiLz48L3N2Zz4=)](https://github.com/kpirnie/streamforge/commits/main)
[![License: MIT](https://img.shields.io/badge/License-MIT-orange.svg?style=for-the-badge&logo=opensourceinitiative&logoColor=white&labelColor=000)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-Join-blue?logo=discord&logoColor=white&style=for-the-badge&labelColor=000)](https://discord.gg/bd4Qan3PaN)
[![Kevin Pirnie](https://img.shields.io/badge/-KevinPirnie.com-000d2d?style=for-the-badge&labelColor=000&logoColor=white&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSJ3aGl0ZSIgc3Ryb2tlLXdpZHRoPSIxLjgiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+CiAgPGNpcmNsZSBjeD0iMTIiIGN5PSIxMiIgcj0iMTAiLz4KICA8ZWxsaXBzZSBjeD0iMTIiIGN5PSIxMiIgcng9IjQuNSIgcnk9IjEwIi8+CiAgPGxpbmUgeDE9IjIiIHkxPSIxMiIgeDI9IjIyIiB5Mj0iMTIiLz4KICA8bGluZSB4MT0iNC41IiB5MT0iNi41IiB4Mj0iMTkuNSIgeTI9IjYuNSIvPgogIDxsaW5lIHgxPSI0LjUiIHkxPSIxNy41IiB4Mj0iMTkuNSIgeTI9IjE3LjUiLz4KPC9zdmc+Cg==)](https://kevinpirnie.com/)


Stream Forge is a Linux terminal application for administering Dispatcharr and Emby, downloading Xtream series, transcoding media, and generating `.strm` companions. Run it with no arguments in a terminal for the interactive menu. Pass a command for a single operation, a script, or a cron job.

## Quick start with Compose

From a checkout of the branch that contains the application, create the host directories and copy the examples.

~~~sh
mkdir -p config working/input working/output
cp .env.example config/.env
# For Xtream only
cp providers.json.example config/providers.json

# Edit the files under config, then open the interactive menu
export PUID=$(id -u) PGID=$(id -g)
docker compose pull
docker compose run --rm streamforge

# Run one command without a TTY
docker compose run --rm -T streamforge dispatcharr export
~~~

Start with [compose.yaml](compose.yaml), [.env.example](.env.example), and [providers.json.example](providers.json.example). Enable only the services you use. For FFmpeg and FFprobe, see [Media tools and GPU access](#media-tools-and-gpu-access).

The Compose example uses `ghcr.io/kpirnie/streamforge:dev`. The build workflow publishes `dev` from `develop` and `latest` from `main`. Set `STREAMFORGE_IMAGE` to select a published tag. Both builds target Linux `amd64` and `arm64`.

## Docker and Podman

Use the same configuration and working directories with either runtime. Match the container user to the host account that owns those directories. The image defaults to UID and GID 1000.

~~~sh
image=ghcr.io/kpirnie/streamforge:dev

# Docker interactive menu
docker run --rm -it --read-only --user "$(id -u):$(id -g)" \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" "$image"

# Docker single command
docker run --rm --read-only --user "$(id -u):$(id -g)" \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" \
  "$image" dispatcharr export

# Rootless Podman interactive menu
podman run --rm -it --read-only --userns=keep-id --user "$(id -u):$(id -g)" \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" "$image"

# Rootless Podman single command
podman run --rm --read-only --userns=keep-id --user "$(id -u):$(id -g)" \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" \
  "$image" diagnostics --json
~~~

For rootless Podman with the `podman-compose` provider, match the host identity and run without a shared pod:

~~~sh
export PUID=$(id -u) PGID=$(id -g) STREAMFORGE_USERNS_MODE=keep-id
podman compose --in-pod=false run --rm streamforge
podman compose --in-pod=false run --rm -T streamforge diagnostics --json
~~~

Use `:ro,Z` and `:rw,Z` on bind mounts that need private SELinux labels. Use `:z` for directories intentionally shared by multiple containers. Stream Forge makes outbound connections and needs no published ports.

## Native Linux CLI

Build using Go 1.26 or newer. Native runs use the same fixed `/config` and `/working` paths as containers. Create or mount those directories on the host and give the running account read access to configuration and write access to the working tree.

~~~sh
CGO_ENABLED=0 go build -trimpath -o streamforge .
./streamforge --help
./streamforge
./streamforge dispatcharr export
~~~

Run the binary with no arguments in a TTY to open the menu. No arguments without a TTY prints usage and exits. For container commands, replace `./streamforge` with `docker compose run --rm -T streamforge` or put the subcommand after the image name in `docker run` or `podman run`.

## Files and permissions

| Path | Contents |
| --- | --- |
| `/config/.env` | Optional service credentials and application settings. |
| `/config/providers.json` | Read-only Xtream provider definitions. |
| `/working/input` | Source media and completed provider downloads. |
| `/working/output` | Processed media and `.strm` companions. |
| `/working` | `channels.csv`, session logs, and saved diagnostics. |

Mount `/config` read-only and `/working` read/write. Keep credentials in the host configuration directory or pass them through the process environment. Relative media paths such as `shows/Example` resolve under `/working/input`. You can also specify `input/shows/Example` or `/working/input/shows/Example`. The `strm` command accepts either tree. Transcode commands read from `input`.

For playback paths, set `STREAMFORGE_PLAYBACK_PARENT` to the absolute directory the player sees above both trees. For example, `/library` produces `/library/input/shows/episode.mkv` for source media or `/library/output/shows/episode.mkv` for processed media. The player needs those trees available at the paths written inside the companions.

## Configuration

Command-line options override corresponding environment settings. Process environment values override `/config/.env`. The interactive setup offers session values for eligible unset settings. Credentials use the environment, `/config/.env`, or eligible interactive prompts.

| Variable | Purpose |
| --- | --- |
| `DISP_URL`, `DISP_USER`, `DISP_PASS` | Dispatcharr server URL and login. Required for Dispatcharr commands. |
| `EMBY_HOST`, `EMBY_API_KEY` | Emby server URL and API key. Required for Emby commands. |
| `DISP_M3U_URL` | Playlist URL for `emby logos` and `emby all`. |
| `TMDB_API_KEY` | Optional TMDB lookup for companion names. |
| `STREAMFORGE_TIMEOUT` | HTTP timeout, default `30s`. Downloads use it for connection and response stalls. |
| `STREAMFORGE_VERIFY_TLS` | Verify server certificates, default `true`. |
| `STREAMFORGE_USER_AGENT` | HTTP user agent, default `VLC/3.0.20 LibVLC/3.0.20`. |
| `STREAMFORGE_CONFIRM` | Require confirmation for protected actions, default `true`. |
| `STREAMFORGE_START_CHANNEL` | First channel number for renumbering, default `1`. |
| `STREAMFORGE_NORMALIZER` | Optional Go regular expression whose matches are removed from stream names during grouping. |
| `STREAMFORGE_PLAYBACK_PARENT` | Absolute player-visible directory above `input` and `output`. |
| `STREAMFORGE_FFMPEG`, `STREAMFORGE_FFPROBE` | Absolute paths to the executables, default `/usr/local/bin/ffmpeg` and `/usr/local/bin/ffprobe`. |

Use the host or container’s standard proxy environment when needed. Avoid storing secrets in the repository. Compose interpolation for mount paths, user IDs, and the image uses the shell environment or the Compose project’s own `.env`. Stream Forge uses `config/.env` for application settings.

### Xtream providers

Each top-level key in `providers.json` is a provider name. The name shown in search results is also the name passed to `--provider`. Invalid entries are skipped so valid providers remain usable.

| Field | Purpose |
| --- | --- |
| `url`, `username`, `password` | Required provider base URL and login. |
| `priority` | Lower numbers appear first. Names break ties alphabetically. Default `0`. |
| `concurrent` | Maximum simultaneous episode transfers across that provider’s selected series. Default `1`. |
| `retries` | Retry budget for recoverable download failures. Default `2`. |
| `retry_delay` | Initial retry delay as a duration. Default `1s`. |
| `bandwidth_kbps` | Aggregate limit per provider in KB/s. Default `0` for unlimited. |
| `headers` | Optional custom HTTP headers, including a provider-specific `User-Agent`. |

## Command reference

| Option | Effect |
| --- | --- |
| `--help`, `-h` | Print usage without contacting a service. |
| `--verbose`, `-v` | Add debug detail to terminal and session logs. |
| `--dry-run` | Preview using the current data without applying changes. A regular session log is still written. |
| `--yes`, `-y` | Approve protected changes or proceed past the applicable disk-space check. |
| `--timeout=30s`, `--verify-tls=true`, `--user-agent=VALUE` | Override HTTP settings for this run. |
| `--start-channel=1`, `--normalizer=PATTERN` | Override Dispatcharr numbering or grouping settings. |
| `--ffmpeg=PATH`, `--ffprobe=PATH`, `--playback-parent=PATH` | Override media tool or player paths. |

A failed command exits nonzero. Batch commands can preserve completed items and still exit nonzero when some items fail.

### Dispatcharr

| Command | Operation |
| --- | --- |
| `dispatcharr refresh` | Queue an M3U refresh. |
| `dispatcharr create-update` | Group streams by normalized name and create or update matching channels. |
| `dispatcharr prune` | Remove stale source streams and their newly emptied channels and groups. |
| `dispatcharr match` | Update TVG IDs from mapped EPG records. |
| `dispatcharr renumber` | Assign consecutive numbers to nonempty channels in alphabetical order. |
| `dispatcharr reorder` | Sort streams by M3U account name, with unassociated streams last. |
| `dispatcharr clear-epg` | Clear EPG mappings. |
| `dispatcharr export` | Write all channels and TVG IDs to `/working/channels.csv`, replacing the previous export. |
| `dispatcharr all` | Run refresh, create/update, prune, match, renumber, and reorder in that order. |

`all` waits for active accounts that expose refresh status. Export and Clear EPG are separate commands.

~~~sh
streamforge dispatcharr all --dry-run
streamforge dispatcharr all --yes
streamforge dispatcharr renumber --start-channel=101 --yes
streamforge dispatcharr export
~~~

### Emby

| Command | Operation |
| --- | --- |
| `emby refresh` | Refresh the guide and wait for completion. |
| `emby logos` | Update logo variants for channels uniquely matched against the playlist. |
| `emby numbers` | Clear channel numbers and their related field locks. |
| `emby all` | Refresh the guide first, then update logos and clear channel numbers. A failed refresh is reported and channel operations continue. |

~~~sh
streamforge emby all --dry-run
streamforge emby all --yes
~~~

### Xtream series

Search queries all valid providers by default. It shows exact title matches before partial matches within each provider. Use the resulting series ID to list episode IDs or download episodes.

~~~sh
streamforge series search "Example Show"
streamforge series search --provider "Example Provider" "Example Show"
streamforge series episodes --provider "Example Provider" 123
streamforge series download --provider "Example Provider" 123 --episode=456 --episode=457
streamforge series download --provider "Example Provider" 123 --all-episodes --process
streamforge series download \
  --select="Example Provider|123|all" \
  --select="Other Provider|987|321,322"
~~~

Download-only validates each episode with FFprobe and writes its companion. `--process` waits for the download batch, then sends successful downloads through the transcode profile. Originals stay in `input` by default under provider and series directories. Existing episodes are skipped. Use `--redownload --yes` to replace them after validation. A failed replacement preserves the previous file. Recoverable transfer failures can resume within the same run when the provider supports HTTP Range.

### Transcode and generate companions

~~~sh
streamforge movies transcode movies --codec=hevc --container=mkv --quality=balanced
streamforge series transcode shows --convert-hdr --playback-parent=/library
streamforge movies transcode movies/example.mkv --force --delete-original --yes
streamforge strm input/shows --playback-parent=/library
streamforge strm output/movies --exclude='extras,*.sample.mkv'
~~~

Both transcode commands accept a file or scan a directory recursively. Output mirrors the input directory tree. Hidden entries and symlinks are skipped. Exclusions are comma-separated, case-insensitive basename globs.

| Flag | Environment variable | Values or effect |
| --- | --- | --- |
| `--codec=VALUE` | `STREAMFORGE_CODEC` | `keep` (default), `hevc`, `av1`. |
| `--container=VALUE` | `STREAMFORGE_CONTAINER` | `keep` (default), `mkv`, `mp4`. |
| `--quality=VALUE` | `STREAMFORGE_QUALITY` | `balanced` (default), `high`, `compression`. |
| `--hardware=VALUE` | `STREAMFORGE_HARDWARE` | `auto` (default), `cpu`, `nvidia`, `intel`, `amd`. |
| `--jobs=N` | `STREAMFORGE_JOBS` | Override concurrent job count within available capacity. |
| `--resolution=WIDTHxHEIGHT` | `STREAMFORGE_RESOLUTION` | Resize video. Source dimensions are the default. |
| `--frame-rate=FPS` | `STREAMFORGE_FRAME_RATE` | Set frame rate, such as `24` or `24000/1001`. |
| `--aspect=W:H` | `STREAMFORGE_ASPECT` | Override display aspect ratio. |
| `--exclude=GLOBS` | `STREAMFORGE_EXCLUDE` | Skip matching file or directory names. |
| `--force` | `STREAMFORGE_FORCE` | Reprocess and replace corresponding outputs after validation. |
| `--convert-hdr` | `STREAMFORGE_CONVERT_HDR` | Tone-map HDR sources to SDR using CPU filters. |
| `--delete-original` | `STREAMFORGE_DELETE_ORIGINAL` | Remove the source after successful conversion. Use `--yes` to approve. |

CPU jobs default to half the available logical CPUs. GPU encoding defaults to one job, with an explicit job count capped at two. Stream copy retains the original video bitstream. HEVC 10-bit encoding can preserve static HDR metadata. Use stream copy to preserve dynamic HDR metadata. Incompatible stream and container combinations are reported.

Standalone `strm` requires a playback parent. Set it for transcode runs to generate companions for processed output. When TMDB is configured, lookup can change the companion’s filename while the referenced media keeps its filename. Lookups that fail fall back to the media basename.

### Diagnostics and logs

~~~sh
streamforge diagnostics
streamforge diagnostics --json
streamforge diagnostics --json --save
~~~

The report includes build information, runtime, service status, mounted paths, media dependencies, device access, and a sanitized summary of the current session log. `--save` writes the report under `/working` and prints its path. Session logs are named `streamforge-YYYYMMDD-HHMMSS.log` with a suffix when needed.

## Interactive menu

Choose **Continue with Defaults** or enter the settings available for the session. Use arrow keys to move, type to filter the current list, and press Enter to select. Protected actions present a preview. Press Y to proceed or Enter to decline. Results stay visible until you return. Esc goes back or cancels the active operation. Ctrl+C exits.

For series, open **Search providers**, press B to browse the results, choose a series, then press B to select episodes. Toggle episodes individually or as a season and add selections across providers to the **Download queue**. Choose **Download Only** or **Download and Process**. During a transfer, use the arrows and X to cancel one active download, or Esc to cancel the batch.

**System Status** shows connectivity and tool availability and offers **Retry Connections**, **Copy Diagnostics**, and **Save diagnostics**. Clipboard copy uses the terminal when supported and saves a report otherwise.

## Media tools and GPU access

FFmpeg and FFprobe come from the host. Use Linux binaries for the image architecture. Static builds are the simplest to mount. Dynamically linked builds also need a compatible loader and libraries. Enable the commented media mounts in [compose.yaml](compose.yaml), or pass them directly.

~~~sh
docker run --rm --read-only --user "$(id -u):$(id -g)" \
  -v "$PWD/config:/config:ro" -v "$PWD/working:/working:rw" \
  -v /opt/media/bin/ffmpeg:/usr/local/bin/ffmpeg:ro \
  -v /opt/media/bin/ffprobe:/usr/local/bin/ffprobe:ro \
  ghcr.io/kpirnie/streamforge:dev movies transcode movies --codec=hevc
~~~

The same binary mounts work with `podman run` and the rootless user mapping shown above. Choose builds with the encoders you use. SDR conversion requires the `zscale` and `tonemap` filters. Run `diagnostics` to check availability.

For Intel or AMD, pass the applicable `/dev/dri/renderD*` node, the matching user-space driver, and the group that grants device access. The Compose example shows the device and supplementary group entries. For NVIDIA, configure the host’s NVIDIA container integration and expose the GPU using your runtime’s supported method. The selected FFmpeg binary needs the corresponding encoder. Unavailable hardware falls back to CPU when a compatible encoder is present.

## SSH and cron

Allocate a terminal for interactive SSH sessions. Pass an explicit subcommand and disable TTY allocation for scheduled work.

~~~sh
ssh -t media-host 'cd /srv/streamforge && docker compose run --rm streamforge'
~~~

~~~cron
15 3 * * * cd /srv/streamforge && docker compose run --rm -T streamforge dispatcharr export
~~~

## License

Stream Forge is released under the [MIT license](LICENSE).
