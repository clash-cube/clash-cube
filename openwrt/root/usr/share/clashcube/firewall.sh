#!/bin/sh
# fw3 restores interception through the core service, even with management off.
[ -S /var/run/clashcube-core/control.sock ] || exit 0
/usr/bin/clashcube-core firewall-sync
