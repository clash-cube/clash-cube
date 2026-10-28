#!/bin/sh
set -eu

cat > /etc/config/network <<'EOF'
config interface 'loopback'
    option device 'lo'
    option proto 'static'
    option ipaddr '127.0.0.1'
    option netmask '255.0.0.0'

config interface 'wan'
    option device 'eth0'
    option proto 'static'
    option ipaddr '172.30.76.2'
    option ip6addr 'fd30:76::2/64'
    option netmask '255.255.255.0'
    option gateway '172.30.76.1'
    list dns '127.0.0.11'

config interface 'lan'
    option device 'eth1'
    option proto 'static'
    option ipaddr '172.30.77.2'
    option ip6addr 'fd30:77::2/64'
    option netmask '255.255.255.0'
EOF

cat > /etc/config/firewall <<'EOF'
config defaults
    option input 'REJECT'
    option output 'ACCEPT'
    option forward 'REJECT'
    option disable_ipv6 '1'
    option flow_offloading '0'
    option flow_offloading_hw '0'

config zone
    option name 'lan'
    list network 'lan'
    option input 'ACCEPT'
    option output 'ACCEPT'
    option forward 'ACCEPT'

config zone
    option name 'wan'
    list network 'wan'
    option input 'REJECT'
    option output 'ACCEPT'
    option forward 'REJECT'
    option masq '1'
    option mtu_fix '1'

config forwarding
    option src 'lan'
    option dest 'wan'

config rule
    option name 'Lab management'
    option src 'wan'
    option proto 'tcp'
    option dest_port '22 80'
    option target 'ACCEPT'
EOF

uci set dhcp.lan=dhcp
uci set dhcp.lan.interface='lan'
# Docker assigns the test client's address; no DHCP broadcasts are needed.
uci set dhcp.lan.ignore='1'
uci set dhcp.@dnsmasq[0].resolvfile='/tmp/resolv.conf.d/resolv.conf.auto'
uci set dhcp.@dnsmasq[0].localservice='1'
uci set dhcp.@dnsmasq[0].noresolv='0'
uci set uhttpd.main.redirect_https='0'
uci set uhttpd.main.rfc1918_filter='0'
uci set luci.main.mediaurlbase='/luci-static/bootstrap'
uci set openclash.config.enable='0'
uci commit

password=$(cat /run/secrets/root_password)
[ -n "$password" ]
printf '%s\n%s\n' "$password" "$password" | passwd root >/dev/null 2>&1
unset password

touch /etc/clashcube-lab-configured
