# Stream Forge

Stream Forge is a one-shot Linux CLI for media administration. The initial development build implements Dispatcharr commands.

```sh
go build -o streamforge .
./streamforge dispatcharr --help
```

Set `DISP_URL`, `DISP_USER` and `DISP_PASS` in the process environment or in a read-only `/config/.env` file. Environment values take precedence. The CLI writes a timestamped session log under `/working`, including for dry runs.

| Command | Operation |
| --- | --- |
| `dispatcharr export` | Write all channels and TVG IDs to `/working/channels.csv`. |
| `dispatcharr refresh` | Request asynchronous M3U source refresh. |
| `dispatcharr match` | Synchronize channel TVG IDs with their mapped EPG records. |
| `dispatcharr clear-epg` | Clear channel EPG mappings. |
| `dispatcharr renumber` | Number nonempty channels in alphabetical order. |
| `dispatcharr reorder` | Sort each channel's streams by M3U account name. |

Use `--dry-run` to preview an action and `--yes` to authorize changes. Use `--verbose` for debug logs. Export overwrites its CSV automatically.

| Optional environment variable | Default |
| --- | --- |
| `STREAMFORGE_TIMEOUT` | `30s` |
| `STREAMFORGE_START_CHANNEL` | `1` |
| `STREAMFORGE_VERIFY_TLS` | `true` |
| `STREAMFORGE_CONFIRM` | `true` |
| `STREAMFORGE_USER_AGENT` | `VLC/3.0.20 LibVLC/3.0.20` |

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
