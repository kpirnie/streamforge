<h1 align="center"><img src="docs/assets/streamforge-full-logo.png" alt="Stream Forge" width="640"></h1>

[![Build Main](https://img.shields.io/github/actions/workflow/status/kpirnie/streamforge/build.yml?branch=main&label=Main&logoColor=white&logo=github&labelColor=000&style=for-the-badge)](https://github.com/kpirnie/streamforge/actions/workflows/build.yml?query=branch%3Amain)
[![Build Develop](https://img.shields.io/github/actions/workflow/status/kpirnie/streamforge/ci.yml?branch=develop&logoColor=white&label=Develop&logo=github&labelColor=000&style=for-the-badge)](https://github.com/kpirnie/streamforge/actions/workflows/ci.yml?query=branch%3Adevelop)
[![GitHub Issues](https://img.shields.io/github/issues/kpirnie/streamforge?style=for-the-badge&logo=github&color=006400&logoColor=white&labelColor=000)](https://github.com/kpirnie/streamforge/issues)
[![GitHub Security](https://img.shields.io/badge/Security-View-6495ED?link=https://github.com/kpirnie/streamforge/security&logoColor=white&style=for-the-badge&labelColor=000&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgd2lkdGg9IjI0IiBoZWlnaHQ9IjI0IiBmaWxsPSJub25lIiBzdHJva2U9IndoaXRlIiBzdHJva2Utd2lkdGg9IjIiPjxwYXRoIGQ9Ik0xMiAyTDQgNXY2LjA5YzAgNS4wNSAzLjQxIDkuNzYgOCAxMC45MSA0LjU5LTEuMTUgOC01Ljg2IDgtMTAuOTFWNWwtOC0zeiIvPjwvc3ZnPg==)](https://github.com/kpirnie/streamforge/security)
[![Last Commit](https://img.shields.io/github/last-commit/kpirnie/streamforge?style=for-the-badge&labelColor=000&logoColor=white&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSJ3aGl0ZSIgc3Ryb2tlLXdpZHRoPSIxLjgiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+PHJlY3QgeD0iMyIgeT0iNC41IiB3aWR0aD0iMTgiIGhlaWdodD0iMTYuNSIgcng9IjIiLz48bGluZSB4MT0iMyIgeTE9IjkuNSIgeDI9IjIxIiB5Mj0iOS41Ii8+PGxpbmUgeDE9IjgiIHkxPSIyLjUiIHgyPSI4IiB5Mj0iNi41Ii8+PGxpbmUgeDE9IjE2IiB5MT0iMi41IiB4Mj0iMTYiIHkyPSI2LjUiLz48L3N2Zz4=)](https://github.com/kpirnie/streamforge/commits/main)
[![License: MIT](https://img.shields.io/badge/License-MIT-orange.svg?style=for-the-badge&logo=opensourceinitiative&logoColor=white&labelColor=000)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-Join-blue?logo=discord&logoColor=white&style=for-the-badge&labelColor=000)](https://discord.gg/bd4Qan3PaN)
[![Kevin Pirnie](https://img.shields.io/badge/-KevinPirnie.com-000d2d?style=for-the-badge&labelColor=000&logoColor=white&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgc3Ryb2tlPSJ3aGl0ZSIgc3Ryb2tlLXdpZHRoPSIxLjgiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIgc3Ryb2tlLWxpbmVqb2luPSJyb3VuZCI+CiAgPGNpcmNsZSBjeD0iMTIiIGN5PSIxMiIgcj0iMTAiLz4KICA8ZWxsaXBzZSBjeD0iMTIiIGN5PSIxMiIgcng9IjQuNSIgcnk9IjEwIi8+CiAgPGxpbmUgeDE9IjIiIHkxPSIxMiIgeDI9IjIyIiB5Mj0iMTIiLz4KICA8bGluZSB4MT0iNC41IiB5MT0iNi41IiB4Mj0iMTkuNSIgeTI9IjYuNSIvPgogIDxsaW5lIHgxPSI0LjUiIHkxPSIxNy41IiB4Mj0iMTkuNSIgeTI9IjE3LjUiLz4KPC9zdmc+Cg==)](https://kevinpirnie.com/)


Stream Forge is a terminal application for Linux, macOS, and Windows. It supports administering Dispatcharr and Emby, downloading Xtream movies and series, transcoding media, and generating `.strm` companions. Run it with no arguments in a terminal for the interactive menu. Pass a command for a single operation, a script, or a cron job.

## Install

Download and run the bootstrapper as your regular user. It checks that both `ffmpeg` and `ffprobe` run from `PATH`, then verifies the release checksum and installs StreamForge. Builds for macOS and Windows require FFmpeg tools that include the `fd` protocol.

### Linux and macOS

~~~sh
curl -fsSLO https://raw.githubusercontent.com/kpirnie/streamforge/main/bootstrap.sh
sh bootstrap.sh
~/.local/bin/streamforge
~~~

The shell bootstrapper installs to `~/.local/bin`. Linux binaries are statically linked and work on glibc and musl distributions. macOS builds are available for Intel and Apple silicon.

### Windows

~~~powershell
Invoke-WebRequest https://raw.githubusercontent.com/kpirnie/streamforge/main/bootstrap.ps1 -OutFile bootstrap.ps1
.\bootstrap.ps1
& "$env:LOCALAPPDATA\StreamForge\bin\streamforge.exe"
~~~

The PowerShell bootstrapper installs to `%LOCALAPPDATA%\StreamForge\bin`. It selects the x86, x64, or ARM64 build. Use a workspace on a filesystem that supports hard links, such as NTFS, for atomic media publication.

### Release channels

Set `STREAMFORGE_BIN_DIR` before running either bootstrapper to change the install location.

Pushes to `main` publish prereleases tagged `beta-<commit SHA>`. Pushing a version tag publishes a stable release. The bootstrapper installs the latest stable release by default. Set `STREAMFORGE_RELEASE` to a tag from the [releases page](https://github.com/kpirnie/streamforge/releases) to install a specific stable or beta build.

Run `streamforge` in a terminal to open the interactive menu, or run `streamforge --help` for commands. Windows release executables include the StreamForge icon. The `--icon` option writes the embedded PNG to standard output.

### Build from source

Use the Go version specified in `go.mod`.

~~~sh
CGO_ENABLED=0 go build -trimpath -o streamforge .
./streamforge --help
~~~

For a Windows source build with the Explorer icon, run `go run ./internal/winicon amd64 streamforge_windows_amd64.syso` before building. Use the target architecture in both arguments.

## Files and permissions

The configuration directory is `~/.config/streamforge` on Linux, `~/Library/Application Support/streamforge` on macOS, and `%APPDATA%\streamforge` on Windows. Linux follows `XDG_CONFIG_HOME` when set. Copy [.env.example](.env.example) or [providers.json.example](providers.json.example) there for the services you use. The workspace defaults to `~/StreamForge`. Set `STREAMFORGE_WORK_DIR` in the process environment to an absolute path to use an existing media workspace.

| Path | Contents |
| --- | --- |
| `.env` in the configuration directory | Optional service credentials and application settings. |
| `providers.json` in the configuration directory | Read-only Xtream provider definitions. |
| `~/StreamForge/input` | Source media and completed provider downloads. |
| `~/StreamForge/output` | Processed media and `.strm` companions. |
| `~/StreamForge/download-queue.yaml` | Pending terminal download selections, restored on startup. |
| `~/StreamForge` | `channels.csv`, session logs, and saved diagnostics. |

Keep credentials in the user configuration directory or pass them through the process environment. Relative media paths such as `shows/Example` resolve under `input` in the workspace. You can also pass `input/shows/Example` or its absolute path. The `strm` command accepts both media trees. Transcode commands read from `input`.

For playback paths, set `STREAMFORGE_PLAYBACK_PARENT` to the absolute directory the player sees above both trees. For example, `/library` produces `/library/input/shows/episode.mkv` for source media or `/library/output/shows/episode.mkv` for processed media. The player needs those trees available at the paths written inside the companions.

## Configuration

Command-line options override corresponding environment settings. Process environment values override `.env` in the configuration directory. The interactive setup offers session values for eligible unset settings. Credentials use the environment, `.env`, or eligible interactive prompts.

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
| `STREAMFORGE_FFMPEG`, `STREAMFORGE_FFPROBE` | Absolute paths to the executables. Both default to discovery on `PATH`. |

Use the standard proxy environment variables when needed. Keep configuration secrets outside the repository.

### Xtream providers

Each top-level key in `providers.json` is a provider name. The name shown in search results is also the name passed to `--provider`. Invalid entries are skipped so valid providers remain usable.

| Field | Purpose |
| --- | --- |
| `url`, `username`, `password` | Required provider base URL and login. |
| `priority` | Lower numbers appear first. Names break ties alphabetically. Default `0`. |
| `concurrent` | Maximum simultaneous movie and episode transfers per provider. Default `1`. |
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
| `dispatcharr export` | Write all channels and TVG IDs to `~/StreamForge/channels.csv`, replacing the previous export. |
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

Download-only validates each new episode with FFprobe and writes its companion. `--process` waits for the download batch, then sends successful downloads through the transcode profile. Series companions and processed videos go under `output/<Series Name>/Season NN/`. Originals stay in `input` by default under provider and series directories. Download-only refreshes companions for existing episodes without transferring the videos again. Use `--redownload --yes` to replace videos after validation. A failed replacement preserves the previous file. Recoverable transfer failures can resume within the same run when the provider supports HTTP Range.

### Xtream movies

Search for a movie and use the provider name and movie ID from the results to download it. Add `--process` to apply the transcode profile after downloading. Movie companions and processed videos go under `output/Movies/<Movie Name (Year)>/` when the provider supplies a year.

```bash
streamforge movies search "Example Movie"
streamforge movies download --movie="Example Provider|123" --process
```

### Transcode and generate companions

~~~sh
streamforge movies transcode movies
streamforge series transcode shows --playback-parent=/library
streamforge movies transcode movies/example.mkv --force --delete-original --yes
streamforge strm input/shows --playback-parent=/library
streamforge strm output/movies --exclude='extras,*.sample.mkv'
~~~

Movie transcoding defaults to HEVC in MKV with audio copied, hardware encoding where available, and a libx265 fallback. A matching `.srt` beside the movie is embedded as English SubRip in MKV or as `mov_text` in MP4. Series transcoding keeps the source container and prefers hardware HEVC at QP 26, with a CPU fallback. It tone-maps HDR to SDR and skips episodes under 500 MiB, episodes already in HEVC or AV1, and encodes that do not reduce file size. `--force` bypasses the series size and codec skips. Download processing selects the profile from the media type.

Both transcode commands accept a file or scan a directory recursively. Output mirrors the input directory tree. Hidden entries and symlinks are skipped. Exclusions are comma-separated, case-insensitive basename globs.

| Flag | Environment variable | Values or effect |
| --- | --- | --- |
| `--codec=VALUE` | `STREAMFORGE_CODEC` | `hevc` (default), `keep`, `av1`. |
| `--container=VALUE` | `STREAMFORGE_CONTAINER` | `keep`, `mkv`, `mp4`. Defaults to MKV for movies and source container for series. |
| `--quality=VALUE` | `STREAMFORGE_QUALITY` | `script` (default), `balanced`, `high`, `compression`. |
| `--hardware=VALUE` | `STREAMFORGE_HARDWARE` | `auto` (default), `cpu`, `nvidia`, `intel`, `amd`. |
| `--jobs=N` | `STREAMFORGE_JOBS` | Cap concurrent transcoding jobs and their aggregate CPU use in logical CPUs. |
| `--resolution=WIDTHxHEIGHT` | `STREAMFORGE_RESOLUTION` | Resize video. Source dimensions are the default. |
| `--frame-rate=FPS` | `STREAMFORGE_FRAME_RATE` | Set frame rate, such as `24` or `24000/1001`. |
| `--aspect=W:H` | `STREAMFORGE_ASPECT` | Override display aspect ratio. |
| `--exclude=GLOBS` | `STREAMFORGE_EXCLUDE` | Skip matching file or directory names. |
| `--force` | `STREAMFORGE_FORCE` | Reprocess and replace corresponding outputs after validation. |
| `--convert-hdr` | `STREAMFORGE_CONVERT_HDR` | Tone-map HDR sources to SDR using CPU filters. |
| `--delete-original` | `STREAMFORGE_DELETE_ORIGINAL` | Remove the source after successful conversion. Use `--yes` to approve. |

Jobs default to half the CPU capacity reported by Go, with a minimum of one. Linux confines media commands to a shared CPU affinity set. Windows applies the shared CPU budget through a Job Object. macOS accepts a transcode job budget at least as large as the host’s logical CPU count. Set `STREAMFORGE_JOBS` to the output of `sysctl -n hw.logicalcpu` to enable transcoding. Smaller budgets fail before FFmpeg starts because macOS does not provide the same hard CPU cap. Stream copy retains the original video bitstream. HEVC 10-bit encoding can preserve static HDR metadata. Use stream copy to preserve dynamic HDR metadata. Incompatible stream and container combinations are reported.

Standalone `strm` requires a playback parent. Set it for transcode runs to generate companions for processed output. When TMDB is configured, lookup can change the companion’s filename while the referenced media keeps its filename. Lookups that fail fall back to the media basename.

### Diagnostics and logs

~~~sh
streamforge diagnostics
streamforge diagnostics --json
streamforge diagnostics --json --save
~~~

The report includes build information, runtime, service status, application directories, media dependencies, device access, and a sanitized summary of the current session log. `--save` writes the report under `~/StreamForge` and prints its path. Session logs are named `streamforge-YYYYMMDD-HHMMSS.log` with a suffix when needed.

## Interactive menu

Choose **Continue with Defaults** or enter the settings available for the session. Use arrow keys to move, type to filter the current list, and press Enter to select. Protected actions present a preview. Press Y to proceed or Enter to decline. Results stay visible until you return. Esc goes back or cancels the active operation. Ctrl+C exits.

Open **Downloads** and choose **Search movies** or **Search series**. The results show providers with match counts. Choose a provider to see its matching titles. Select a movie to toggle it in the queue. For a series match, choose **Add All Episodes to Download Queue** or toggle individual episodes and seasons. The shared **Download queue** lets you review or remove selections before choosing **Download Only** or **Download and Process**. If some providers fail, the provider menu still opens for available matches. During a transfer, use the arrows and X to cancel one active download, or Esc to cancel the batch.

StreamForge saves queue changes in `~/StreamForge/download-queue.yaml`. Items leave the queue when their video and playback link are ready, including processing if selected. Failed or cancelled items stay queued so you can retry them. The completion summary shows the batch outcome and the video, playback link, and player paths for completed items.

**System Status** shows connectivity and tool availability and offers **Retry Connections**, **Copy Diagnostics**, and **Save diagnostics**. Clipboard copy uses the terminal when supported and saves a report otherwise.

## Media tools and GPU access

FFmpeg and FFprobe are required for downloads and transcoding. Install both on `PATH`, or configure their absolute paths with `STREAMFORGE_FFMPEG` and `STREAMFORGE_FFPROBE`. FFmpeg needs the encoders you use and the `zscale` and `tonemap` filters for SDR conversion. Run `streamforge diagnostics` to check availability.

On Linux, for Intel and AMD hardware encoding, the account running StreamForge needs access to the applicable `/dev/dri/renderD*` device and the matching driver. NVIDIA encoding requires a compatible driver and an accessible `/dev/nvidiactl`. StreamForge uses software encoding on macOS and Windows. If Linux hardware encoding is unavailable, StreamForge can fall back to a compatible CPU encoder.

## SSH and cron

Allocate a terminal for interactive SSH sessions. Pass an explicit subcommand and disable TTY allocation for scheduled work.

~~~sh
ssh -t media-host '~/.local/bin/streamforge'
~~~

~~~cron
15 3 * * * "$HOME/.local/bin/streamforge" dispatcharr export
~~~

## License

Stream Forge is released under the [MIT license](LICENSE).
