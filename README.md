# Explo - Music Discovery for Self-Hosted Music Systems

**Explo** bridges the gap between automated music discovery and self-hosted libraries. It fetches personalized recommendations from ListenBrainz and places the tracks directly into your private media library.

This repository is a fork of the original [LumePart/Explo](https://github.com/LumePart/Explo).

### Key Differences
- **Web UI Stripped**: Removed the web interface, React frontend, and backend server (CLI only).
- **Enhanced Download Services**: Added direct, native Qobuz downloads (`qobuz`) and Squidwtf Qobuz downloads (`squidwtf-qobuz`).

---

## Getting Started

### Prerequisites

To build and run Explo from source, you will need:
1. **Go** (version 1.24 or higher)
2. **FFmpeg** installed and added to your system `$PATH` (used for metadata writing and audio conversion)
3. **Python 3** with `ytmusicapi` installed (required only if using YouTube Music search):
   ```bash
   pip install ytmusicapi
   ```
4. A **ListenBrainz** account with scrobbling history (recommendations are personalized).

### Compilation

Build the Explo binary directly from the root of the repository:
```bash
go build -o explo ./src/main/
```

This compiles a standalone executable `explo` in the root folder.

---

## Usage

Run the Explo CLI tool using command-line flags and environment variables loaded from a `.env` file:

```bash
./explo -c <config-path> --playlist <playlist-type> [flags]
```

### Command-Line Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config` | `-c` | `.env` | Path to the configuration `.env` file. |
| `--playlist` | `-p` | `weekly-exploration` | The ListenBrainz playlist to fetch (`weekly-exploration`, `weekly-jams`, `daily-jams`, `on-repeat`). |
| `--download-mode` | `-d` | `normal` | `normal` downloads what is missing locally, `skip` only uses what is already there, `force` always downloads. |
| `--replace-playlist` | | `true` | Replace the existing playlist of the same name. Set to `false` to keep a history instead, which date-stamps each run's playlist. |
| `--clean-downloads` | | `false` | Delete previously downloaded tracks before downloading new ones. Requires `USE_SUBDIRECTORY=true`. |
| `--exclude-local`| `-e` | `false` | Skip downloading tracks that are already present in your target library. |
| `--refresh-only` | | `false` | Trigger a library rescan and exit, without discovering or downloading anything. |
| `--search-mbid` | | | Resolve a MusicBrainz recording ID through ListenBrainz, report whether your library already holds it, and exit. A diagnostic for matching problems. |
| `--version` | `-v` | | Print the version and exit. |

`--persist` is deprecated. It means the opposite of `--replace-playlist` and is still
honoured, so `--persist=true` is applied as `--replace-playlist=false`. The same goes
for the `PERSIST` variable, superseded by `REPLACE_PLAYLIST`.

Two things changed from the previous behaviour:

- Playlists are now replaced in place and carry a stable name (`Weekly Exploration`)
  rather than a date-stamped one (`Weekly Exploration Week30`). Pass
  `--replace-playlist=false` to keep a dated history instead.
- Deleting previously downloaded tracks is no longer bundled with replacing the
  playlist. `--persist=false` used to do both; pass `--clean-downloads` alongside
  `--replace-playlist` to keep deleting them. Explo warns if it detects the old
  setting without the new flag.

### Running a Manual Run

To manually trigger a weekly exploration run:
```bash
./explo -c .env --playlist weekly-exploration
```

---

## Configuration Reference

Explo is configured entirely via environment variables. Create a `.env` file in your working directory (copied from the `sample.env` template) and configure the variables described below.

### Music Discovery Settings
- `DISCOVERY_SERVICE`: The recommendation service to use. Only `listenbrainz` is supported.
- `LISTENBRAINZ_USER`: Your ListenBrainz username.
- `LISTENBRAINZ_DISCOVERY`: `playlist` (default) to fetch full weekly exploration lists, or `api` for testing.

### Music Server Settings (`EXPLO_SYSTEM`)
- `EXPLO_SYSTEM`: The target self-hosted music system. Options: `plex`, `jellyfin`, `emby`, `subsonic`, `mpd`.
- `SYSTEM_URL`: Address of your music system (e.g. `http://192.168.1.100:4533`).
- `SYSTEM_USERNAME`: Login username for the server.
- `SYSTEM_PASSWORD`: Login password (required for Subsonic, Plex).
- `API_KEY`: API Key (required for Emby and Jellyfin, optional for Plex).
- `LIBRARY_NAME`: Name of the target music library (Emby, Jellyfin, Plex).
- `PLAYLIST_DIR`: Local output directory for playlist files (required only for MPD).

### Downloader Settings
- `DOWNLOAD_DIR`: Path to the directory where Explo should save downloaded audio files. In Docker environments, this is mapped via volumes.
- `DOWNLOAD_SERVICES`: A comma-separated list of downloaders in priority order (e.g., `qobuz,slskd,youtube`). Options:
  - `youtube`
  - `slskd`
  - `qobuz` (Direct native downloader using your own Qobuz credentials)
  - `squidwtf-qobuz` (Proxy-based captcha-solving downloader)
- `KEEP_PERMISSIONS`: `true` (default) to keep the original file permissions when migrating downloads.
- `RENAME_TRACK`: `true` to rename downloaded files into `{Title} - {Artist}.[ext]` format instead of the downloader defaults.
- `USE_SUBDIRECTORY`: `true` (default) to place downloads inside a subfolder named after the playlist.

### YouTube Downloader Config
- `YOUTUBE_API_KEY`: Optional YouTube Data API v3 key to improve YouTube search results.
- `TRACK_EXTENSION`: Audio format extension (e.g., `opus`, `mp3`, `m4a`) (default: `opus`).
- `COOKIES_PATH`: Path to a `cookies.txt` file to prevent YouTube downloader bot blocks.

### Slskd (Soulseek) Downloader Config
- `SLSKD_URL`: The API address of your running Slskd instance.
- `SLSKD_API_KEY`: The API key generated by your Slskd instance.
- `MIGRATE_DOWNLOADS`: Set `true` to let Explo move successfully downloaded files from the Slskd download directory to the target music library.
- `SLSKD_DIR`: The download directory configured on your Slskd instance.
- `EXTENSIONS`: Comma-separated preferred file extensions (e.g., `flac,mp3`).
- `MIN_BIT_DEPTH`: Minimum bit depth filter for FLAC files (e.g., `16` or `24`).
- `MIN_BITRATE`: Minimum bitrate filter for MP3 files (e.g., `256` or `320`).

### Native Qobuz Config
The direct native `qobuz` downloader logs directly into Qobuz, dynamically extracts player secrets, and requests files without utilizing third-party captcha proxies.
- `QOBUZ_USER_AUTH_TOKEN`: Your Qobuz user authentication token.
- `QOBUZ_USER_ID`: Your Qobuz user ID.
  > [!TIP]
  > To find these values:
  > 1. Log into your account on [play.qobuz.com](https://play.qobuz.com).
  > 2. Open your browser DevTools (F12) and go to **Application** -> **Local Storage**.
  > 3. Search for the `localuser` key and copy `id` and `userAuthToken` values.
- `QOBUZ_QUALITY`: The audio format to retrieve:
  - `27` = FLAC 24-bit / up to 192kHz (Hi-Res)
  - `7` = FLAC 24-bit / up to 96kHz
  - `6` = FLAC 16-bit / 44.1kHz (CD Quality)
  - `5` = MP3 320kbps

---

## Docker Integration & Automation

Get Explo running with Docker.

### Docker Compose Setup

```yaml
version: '3.8'

services:
  explo:
    build: .
    container_name: explo
    volumes:
      - /path/to/music/library/explo:/data/
      # - /path/to/slskd/downloads:/slskd/ # if using Slskd migration
      - ./.env:/opt/explo/.env:ro
    environment:
      - TZ=Europe/Rome
      - PUID=1000
      - PGID=1000
      - WEEKLY_EXPLORATION_SCHEDULE=0 3 * * 2
      - WEEKLY_EXPLORATION_FLAGS=--playlist=weekly-exploration --replace-playlist=false
    restart: unless-stopped
```

### Automation & Schedules

The Docker container entrypoint script (`docker/start.sh`) automatically scans for active schedule variables in your environment at startup:
- `WEEKLY_EXPLORATION_SCHEDULE`: Standard cron expression (e.g., `0 3 * * 2` for every Tuesday at 3:00 AM).
- `WEEKLY_EXPLORATION_FLAGS`: Executable flags to pass to the binary during execution (e.g., `--playlist=weekly-exploration`).
- `WEEKLY_JAMS_SCHEDULE` / `WEEKLY_JAMS_FLAGS`
- `DAILY_JAMS_SCHEDULE` / `DAILY_JAMS_FLAGS`

When the container launches, it dynamically maps these schedules into the system crontab (`/etc/crontabs/root`), running Explo headless in the background without needing a permanent backend service.

---

## Contributing & License

Contributions, bug reports, and feature requests are welcome! Please open an issue or submit a pull request.

Explo is released under the **GNU General Public License v3**. Please review the `LICENSE` file for full terms and conditions.
