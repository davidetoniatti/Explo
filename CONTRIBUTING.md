# Contributing to Explo

Thank you for your interest in contributing to Explo!

## Development Environment

1.  **Go**: Ensure you have Go 1.24+ installed.
2.  **Node.js**: Node.js 20+ is required for the frontend.
3.  **Python**: Python 3.12+ is required for some downloaders.

## Project Structure

- `src/client/`: Music system clients (Emby, Jellyfin, Plex, etc.).
- `src/downloader/`: Track downloaders (YouTube, Soulseek, Qobuz).
- `src/discovery/`: ListenBrainz discovery logic.
- `src/web/`: React frontend and Go backend for the UI.

## Adding a New Music System

1.  Implement the `APIClient` interface in `src/client/client.go`.
2.  Add a new file in `src/client/` (e.g., `mysystem.go`).
3.  Register the new system in `NewClient` in `src/client/client.go`.

## Adding a New Downloader

1.  Implement the `Downloader` interface (see `src/downloader/downloader.go`).
2.  Add a new file in `src/downloader/`.
3.  Register it in `NewDownloader`.

## Testing

Run all tests with:

```bash
go test ./src/...
```

Please add tests for any new features or bug fixes.

## Pull Request Process

1.  Create a new branch for your changes.
2.  Ensure tests pass and code is formatted (`go fmt`).
3.  Submit a PR with a clear description of the changes.
