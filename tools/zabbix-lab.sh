#!/bin/sh
# Disposable Zabbix 7.0 lab for testing Vogter against a real server.
# The web front end binds to 127.0.0.1 only; nothing is reachable off this host.
#
#   tools/zabbix-lab.sh up      start the containers
#   tools/zabbix-lab.sh feed    seed hosts, then keep problems opening and resolving
#   tools/zabbix-lab.sh down    remove the containers, network and volumes
#
# Then: bin/vogter -c tools/zabbix-lab.toml
set -eu

net=vogter-lab
tag=alpine-7.0-latest
port=18080
dbpass=vogter-lab-only

case "${1:-}" in
  up)
    docker network inspect "$net" >/dev/null 2>&1 || docker network create "$net" >/dev/null
    docker run -d --name vl-db --network "$net" \
      -e POSTGRES_USER=zabbix -e POSTGRES_PASSWORD="$dbpass" -e POSTGRES_DB=zabbix \
      -v vl-db:/var/lib/postgresql/data \
      postgres:16-alpine
    docker run -d --name vl-server --network "$net" \
      -e DB_SERVER_HOST=vl-db -e POSTGRES_USER=zabbix -e POSTGRES_PASSWORD="$dbpass" -e POSTGRES_DB=zabbix \
      "zabbix/zabbix-server-pgsql:$tag"
    docker run -d --name vl-web --network "$net" \
      -p "127.0.0.1:$port:8080" \
      -e DB_SERVER_HOST=vl-db -e POSTGRES_USER=zabbix -e POSTGRES_PASSWORD="$dbpass" -e POSTGRES_DB=zabbix \
      -e ZBX_SERVER_HOST=vl-server -e PHP_TZ=UTC \
      "zabbix/zabbix-web-nginx-pgsql:$tag"
    docker run -d --name vl-agent --network "$net" \
      -e ZBX_HOSTNAME="Zabbix server" -e ZBX_SERVER_HOST=vl-server \
      -v vl-data:/data \
      "zabbix/zabbix-agent2:$tag"
    ;;
  feed)
    exec python3 -I "$(dirname "$0")/zabbix-lab-feed.py"
    ;;
  down)
    docker rm -f vl-agent vl-web vl-server vl-db 2>/dev/null || true
    docker volume rm vl-db vl-data 2>/dev/null || true
    docker network rm "$net" 2>/dev/null || true
    ;;
  *)
    echo "usage: $0 up|feed|down" >&2
    exit 2
    ;;
esac
