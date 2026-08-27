cd projects/Hatcheck-Go/
# Usage: ./restore.sh backups/hatcheck-backup-YYYYMMDD-HHMMSS.tar.gz
# Stop the server first. This replaces objects/ and metadata/ entirely —
# anything written since the backup was taken is gone after this runs.
rm -rf objects metadata
tar xzf "$1"
