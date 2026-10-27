#!/bin/sh
set -eu

if [ ! -f /etc/clashcube-lab-configured ]; then
    # Limit the bundled services on first boot, then preserve services added
    # during plugin development across subsequent restarts.
    for link in /etc/rc.d/S*; do
        service=${link##*/S??}
        case "$service" in
            boot|system|log|rpcd|dnsmasq|dropbear|firewall|network|uhttpd|ucitrack|done) ;;
            *) rm -f "$link" ;;
        esac
    done

    # Vendor first-boot customisations overwrite networking and package feeds.
    # Retain the base migrations and LuCI setup needed by the lab instead.
    for file in /etc/uci-defaults/*; do
        case "${file##*/}" in
            00_uhttpd_ubus|10_migrate-shadow|13_fix-group-user|14_migrate-dhcp-release|15_odhcpd|30_luci-theme-*|50-dnsmasq-*|50-migrate-rpcd-*|50_luci-mod-admin-full|luci-base|luci-i18n-base-*|luci-i18n-firewall-*) ;;
            *) rm -f "$file" ;;
        esac
    done
    cp /lab/configure.sh /etc/uci-defaults/99zz-clashcube-lab
fi

exec /sbin/init
