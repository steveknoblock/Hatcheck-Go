cd projects/Hatcheck-Go/
mkdir -p backups
tar czf backups/hatcheck-backup-$(date +%Y%m%d-%H%M%S).tar.gz objects metadata
