#!/bin/sh

# Set UID/GID from environment or defaults
PUID=${PUID:-1000}
PGID=${PGID:-1000}

echo "[setup] Adjusting permissions for UID $PUID and GID $PGID..."

# Create group if it doesn't exist
if ! getent group explo >/dev/null; then
    groupadd -g "$PGID" explo
fi

# Create user if it doesn't exist
if ! getent passwd explo >/dev/null; then
    useradd -u "$PUID" -g "$PGID" -d /opt/explo -s /bin/sh explo
fi

# Ensure /opt/explo and its contents are owned by the user
chown -R explo:explo /opt/explo

# Try to chown the data directory if it's mounted
if [ -d "/data" ]; then
    chown explo:explo /data
fi

# If user incorectly mounts the config path as a directory, we'll try to automatically append it to .env inside it instead of failing.
# CFG_PATH replaces the old WEB_CFG_PATH name (kept as a fallback for backwards compatibility).
CFG_PATH="${CFG_PATH:-${WEB_CFG_PATH:-/opt/explo/.env}}"
if [ -d "$CFG_PATH" ]; then
    CFG_PATH="$CFG_PATH/.env"
    echo "[setup] Config path is a directory, using $CFG_PATH"
fi

echo "[setup] Initializing cron jobs..."

# Load *_SCHEDULE and *_FLAGS from .env if not already set in the environment,
# so schedules can be configured by editing the mounted .env file directly.
_cfg="${CFG_PATH:-/opt/explo/.env}"
if [ -f "$_cfg" ]; then
  while IFS= read -r _line; do
    case "$_line" in \#*|'') continue ;; esac
    _key="${_line%%=*}"
    case "$_key" in
      *_SCHEDULE|*_FLAGS)
        if [ -z "$(printenv "$_key" 2>/dev/null)" ]; then
          export "$_key=${_line#*=}"
        fi
        ;;
    esac
  done < "$_cfg"
fi

# Clear existing crontab
> /etc/crontabs/root

# $CRON_SHCEDULE was deprecated in v0.11.0, keeping this block for backwards compatibility
if [ -n "$CRON_SCHEDULE" ]; then
    echo "$CRON_SCHEDULE apk add --upgrade yt-dlp && cd /opt/explo && su-exec explo ./explo >> /proc/1/fd/1 2>&1" > /etc/crontabs/root
    chmod 600 /etc/crontabs/root
    echo "[setup] Registered single CRON_SCHEDULE job: $CRON_SCHEDULE"
fi

# Loop over all *_SCHEDULE environment variables
for var in $(env | grep "_SCHEDULE=" | cut -d= -f1); do
  job="${var%_SCHEDULE}"                     # Job name (e.g WEEKLY_EXPLORATION)
  schedule="$(printenv "$var")"              # Cron schedule
  flags_var="${job}_FLAGS"
  flags="$(printenv "$flags_var")"           # e.g. --playlist weekly-exploration

  if [ -z "$schedule" ]; then
    echo "[setup] Skipping $job: schedule is empty"
    continue
  fi

  # Default: just run explo if flags are empty
  # Note: explo runs as the specified user via su-exec
  cmd="cd /opt/explo && su-exec explo ./explo $flags >> /proc/1/fd/1 2>&1"

  echo "$schedule $cmd" >> /etc/crontabs/root
  echo "[setup] Registered job: $job"
  echo "        Schedule: $schedule"
  echo "        Command : ./explo $flags"
done

chmod 600 /etc/crontabs/root

echo "[setup] Starting cron..."

if [ "$EXECUTE_ON_START" = "true" ]; then
    echo "[setup] Executing startup task..."  
    cd /opt/explo && su-exec explo ./explo $START_FLAGS
fi

crond -f -l 2
