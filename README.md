# Explo - Music Discovery CLI for Self-Hosted Libraries

**Explo** is a self-hosted, lightweight Go-based command-line utility designed to bridge the gap between automated music discovery and personal media libraries. 

This project is a CLI-focused fork of the original [LumePart/Explo](https://github.com/LumePart/Explo) repository. While the original project provides a web UI, this version is streamlined to run strictly as a pure CLI application, suited for automated execution via system cron daemons (`crond`), terminal scripts, or headless Docker containers.

Its primary function is to act as a self-hosted alternative to Spotify’s *Discover Weekly* or *Daily Mixes*, pulling personalized recommendations based on your listening habits and placing the tracks directly into your private media library.

---

## Features

- **Personalized ListenBrainz Discovery**: Dynamically retrieves your custom ListenBrainz lists (controlled by runtime flags):
  - `weekly-exploration` (Weekly Exploration recommendations)
  - `weekly-jams` (Weekly Jams)
  - `daily-jams` (Daily Jams)
- **Multi-Downloader Integration Support**:
  - **YouTube Music**: Queries YouTube Music via a Python helper script running `ytmusicapi` and downloads audio via `yt-dlp`.
  - **Soulseek**: Integrates with [Slskd](https://github.com/slskd/slskd) for high-quality peer-to-peer audio retrieval.
  - **Qobuz Direct**: Direct, native Go integration scraping the Qobuz web player bundle to dynamically fetch signing keys and stream high-quality FLAC or MP3 files (supporting quality fallbacks up to 24-bit/192kHz).
  - **SquidWTF Qobuz**: Proxy-based Qobuz downloader using a captcha-solving endpoint.
- **Automated Tagging**: Uses `ffmpeg` to write precise audio metadata (artist, title, album name) to YouTube downloads without losing quality.
- **Media System Syncing**: Automatically creates, updates, and structures playlists inside self-hosted servers:
  - **Plex**
  - **Jellyfin**
  - **Emby**
  - **Subsonic** (Navidrome, etc.)
  - **MPD** (writes native `.m3u` files)
- **Dynamic Scheduling**: Scans active schedules at startup and updates the system crontab in container environments.

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
| `--playlist` | | | The ListenBrainz playlist to fetch (`weekly-exploration`, `weekly-jams`, `daily-jams`). |
| `--persist` | | `true` | Maintain a history of previous discovery playlists (keeps previous weeks/days in your media system). |
| `--exclude-local`| | `true` | Skip downloading tracks that are already present in your target library. |

### Running a Manual Run

To manually trigger a weekly exploration run:
```bash
./explo -c .env --playlist weekly-exploration
```

---

## Configuration Reference

Explo is configured entirely via environment variables. Create a `.env` file in your working directory (copied from the `sample.env` template) and configure the variables described below.

### 1. Music Discovery Settings
- `DISCOVERY_SERVICE`: The recommendation service to use. Only `listenbrainz` is supported.
- `LISTENBRAINZ_USER`: Your ListenBrainz username.
- `LISTENBRAINZ_DISCOVERY`: `playlist` (default) to fetch full weekly exploration lists, or `api` for testing.

### 2. Music Server Settings (`EXPLO_SYSTEM`)
- `EXPLO_SYSTEM`: The target self-hosted music system. Options: `plex`, `jellyfin`, `emby`, `subsonic`, `mpd`.
- `SYSTEM_URL`: Address of your music system (e.g. `http://192.168.1.100:4533`).
- `SYSTEM_USERNAME`: Login username for the server.
- `SYSTEM_PASSWORD`: Login password (required for Subsonic, Plex).
- `API_KEY`: API Key (required for Emby and Jellyfin, optional for Plex).
- `LIBRARY_NAME`: Name of the target music library (Emby, Jellyfin, Plex).
- `PLAYLIST_DIR`: Local output directory for playlist files (required only for MPD).

### 3. Downloader Settings
- `DOWNLOAD_DIR`: Path to the directory where Explo should save downloaded audio files. In Docker environments, this is mapped via volumes.
- `DOWNLOAD_SERVICES`: A comma-separated list of downloaders in priority order (e.g., `qobuz,slskd,youtube`). Options:
  - `youtube`
  - `slskd`
  - `qobuz` (Direct native downloader using your own Qobuz credentials)
  - `squidwtf-qobuz` (Proxy-based captcha-solving downloader)
- `KEEP_PERMISSIONS`: `true` (default) to keep the original file permissions when migrating downloads.
- `RENAME_TRACK`: `true` to rename downloaded files into `{Title} - {Artist}.[ext]` format instead of the downloader defaults.
- `USE_SUBDIRECTORY`: `true` (default) to place downloads inside a subfolder named after the playlist.

### 4. YouTube Downloader Config
- `YOUTUBE_API_KEY`: Optional YouTube Data API v3 key to improve YouTube search results.
- `TRACK_EXTENSION`: Audio format extension (e.g., `opus`, `mp3`, `m4a`) (default: `opus`).
- `COOKIES_PATH`: Path to a `cookies.txt` file to prevent YouTube downloader bot blocks.

### 5. Slskd (Soulseek) Downloader Config
- `SLSKD_URL`: The API address of your running Slskd instance.
- `SLSKD_API_KEY`: The API key generated by your Slskd instance.
- `MIGRATE_DOWNLOADS`: Set `true` to let Explo move successfully downloaded files from the Slskd download directory to the target music library.
- `SLSKD_DIR`: The download directory configured on your Slskd instance.
- `EXTENSIONS`: Comma-separated preferred file extensions (e.g., `flac,mp3`).
- `MIN_BIT_DEPTH`: Minimum bit depth filter for FLAC files (e.g., `16` or `24`).
- `MIN_BITRATE`: Minimum bitrate filter for MP3 files (e.g., `256` or `320`).

### 6. Native Qobuz Config
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

Explo excels when deployed inside a container stack where its execution can be automated in the background.

### Docker Compose Setup

Add the Explo service to your `docker-compose.yaml` stack:

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
      - WEEKLY_EXPLORATION_FLAGS=--playlist=weekly-exploration --persist=true
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

*This project is built upon the wonderful core capabilities developed by the authors of [LumePart/Explo](https://github.com/LumePart/Explo).*
