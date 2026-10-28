#!/bin/sh
set -eu
directory=/etc/clashcube-guard
mkdir -p "$directory" /var/run
chmod 700 "$directory"
# Share the same kernel lock as management Apply. A concurrent init reload
# cannot reinstall an older table after a new policy has been acknowledged.
exec 9>"$directory/devices.lock"
flock -x 9
intent="$directory/intent.json"
cache="$directory/guard.nft"
expected=/var/run/clashcube-device-guard.json
[ -f "$intent" ] || { [ ! -f "$cache" ] && exit 0; }
# A complete transaction must have matching durable intent and compiled rules.
revision=$(jsonfilter -i "$intent" -e '@.revision' 2>/dev/null || true)
cached=$(sed -n 's/^# clashcube-device-revision //p' "$cache" 2>/dev/null || true)
if [ -n "$revision" ] && [ "$revision" = "$cached" ] && nft -c -f "$cache"; then
    nft -f "$cache"
    if nft -s -j list table inet clashcube_guard > "$expected.new" 2>/dev/null; then
        chmod 600 "$expected.new"
        mv "$expected.new" "$expected"
    else
        rm -f "$expected" "$expected.new"
    fi
    exit 0
fi
# Interrupted/corrupt saves cannot be interpreted as permission to route.
# Restrict forwarding globally until management can repair the durable intent.
rm -f "$expected"
logger -t clashcube-guard 'Incomplete device policy transaction; emergency forwarding restriction active'
nft -f - <<'NFT'
add table inet clashcube_guard
delete table inet clashcube_guard
table inet clashcube_guard {
 comment "emergency"
 chain forward { type filter hook forward priority -10; policy drop; }
 chain input { type filter hook input priority -10; policy accept;
  iifname "lo" accept
  tcp dport { 22, 80, 443 } accept
  udp sport 68 udp dport 67 accept
  meta l4proto ipv6-icmp icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } accept
  drop
 }
}
NFT
