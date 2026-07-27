cd projects/Hatcheck-Go/
git fetch
git pull
go build ./...
set -a && source .env && set +a
go run ./server
