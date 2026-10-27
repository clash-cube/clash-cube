#!/bin/sh
set -eu
cd "$(dirname "$0")"

docker compose exec -T router ubus call system board
docker compose exec -T router sh -ec '
    test -f /etc/clashcube-lab-configured
    curl -sS --max-time 5 http://127.0.0.1/cgi-bin/luci/ | grep -q "name=\"luci_password\""
'
docker compose exec -T client sh -ec '
    ip -4 route get 1.1.1.1 | grep "via 172.30.77.2"
    nslookup example.com 172.30.77.2
    curl --noproxy "*" -fsSI --max-time 15 https://example.com
'
docker compose exec -T router iptables -t nat -L zone_wan_postrouting -n -v
