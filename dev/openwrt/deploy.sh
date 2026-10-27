#!/bin/sh
# Management updates preserve the separately installed core service and sessions.
set -eu
cd "$(dirname "$0")/../.."
mode=${1:-management}
case "$mode" in management|core|all) ;; *) echo 'Usage: deploy.sh [management|core|all]' >&2; exit 2;; esac
bootstrap=0
if ! docker compose -f dev/openwrt/compose.yaml exec -T router test -x /usr/bin/clashcube-core; then
    bootstrap=1
fi
build_mode=$mode
if [ "$bootstrap" = 1 ]; then build_mode=all; fi
sh dev/openwrt/build.sh "$build_mode"
docker compose -f dev/openwrt/compose.yaml exec -T router sh -eu -s -- "$mode" "$bootstrap" <<'ROUTER'
mode=$1
bootstrap=$2
source=/workspace/clash-cube
# One-time migration: the legacy web process owned the core through stdin.
if [ "$bootstrap" = 1 ] && [ -x /etc/init.d/clashcube ]; then
    echo 'Migrating legacy core ownership (one core restart).'
    /etc/init.d/clashcube stop
fi
mkdir -p /usr/share/clashcube /www/luci-static/resources/clashcube/assets
if [ ! -f /etc/config/clashcube ]; then
    cp "$source/openwrt/root/etc/config/clashcube" /etc/config/clashcube
    # The development data volume predates the real-router persistent path.
    if [ ! -e /etc/clashcube ]; then ln -s /var/lib/clashcube /etc/clashcube; fi
fi
cp "$source/openwrt/root/usr/share/clashcube/storage.sh" /usr/share/clashcube/storage.sh
mkdir -p /lib/upgrade/keep.d
cp "$source/openwrt/root/lib/upgrade/keep.d/clashcube" /lib/upgrade/keep.d/clashcube
if [ "$mode" != management ] || [ "$bootstrap" = 1 ]; then
    if [ -x /etc/init.d/clashcube-core ]; then /etc/init.d/clashcube-core stop; fi
    cp "$source/bin/openwrt/clashcube-core" /usr/bin/clashcube-core.new
    chmod 755 /usr/bin/clashcube-core.new
    mv /usr/bin/clashcube-core.new /usr/bin/clashcube-core
    cp "$source/openwrt/root/etc/init.d/clashcube-core" /etc/init.d/clashcube-core
    chmod 755 /etc/init.d/clashcube-core
    /etc/init.d/clashcube-core enable
fi
if [ "$mode" != management ] || [ "$bootstrap" = 1 ]; then
    # The core service owns its firewall include independently of management.
    cp "$source/openwrt/root/usr/share/clashcube/firewall.sh" /usr/share/clashcube/firewall.sh
    chmod 755 /usr/share/clashcube/firewall.sh
    uci set firewall.clashcube=include
    uci set firewall.clashcube.type=script
    uci set firewall.clashcube.path=/usr/share/clashcube/firewall.sh
    uci set firewall.clashcube.reload=1
    uci commit firewall
    /etc/init.d/clashcube-core start
fi
if [ "$mode" != core ] || [ "$bootstrap" = 1 ]; then
    if [ "$bootstrap" != 1 ] && [ -x /etc/init.d/clashcube ]; then /etc/init.d/clashcube stop; fi
    cp "$source/bin/openwrt/clashcube-web" /usr/bin/clashcube-web.new
    chmod 755 /usr/bin/clashcube-web.new
    mv /usr/bin/clashcube-web.new /usr/bin/clashcube-web
    cp "$source/openwrt/root/etc/init.d/clashcube" /etc/init.d/clashcube
    cp "$source/openwrt/root/etc/init.d/clashcube-update" /etc/init.d/clashcube-update
    cp "$source/openwrt/root/usr/lib/lua/luci/controller/clashcube.lua" /usr/lib/lua/luci/controller/clashcube.lua
    cp "$source/openwrt/root/usr/lib/lua/luci/view/clashcube.htm" /usr/lib/lua/luci/view/clashcube.htm
    sh "$source/dev/openwrt/install-assets.sh" "$source/bin/openwrt/www" \
        /www/luci-static/resources/clashcube /usr/share/clashcube
    chmod 755 /etc/init.d/clashcube
    chmod 755 /etc/init.d/clashcube-update
    rm -f /tmp/luci-indexcache /tmp/luci-indexcache.*
    # Active LuCI requests can recreate files during cache invalidation. Keep
    # the shared directory so they cannot make a recursive rmdir abort deployment.
    if [ -d /tmp/luci-modulecache ]; then
        find /tmp/luci-modulecache -type f -exec rm -f {} \;
    fi
    uci set uhttpd.main.max_requests=16
    uci commit uhttpd
    /etc/init.d/clashcube enable
    /etc/init.d/clashcube start
fi
ROUTER
echo 'ClashCube: http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube'
