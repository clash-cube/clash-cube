#!/bin/sh
# Sourced by both services; settings live on flash, disposable data may not.
clashcube_storage() {
    . /lib/functions.sh
    config_load clashcube
    config_get CLASHCUBE_HOME main data_dir /etc/clashcube
    config_get clashcube_storage_mode main storage_mode persistent
    config_get clashcube_external_mount main external_mount /mnt/clashcube
    case "$CLASHCUBE_HOME" in /*) ;; *) echo 'ClashCube data_dir must be absolute' >&2; return 1;; esac
    CLASHCUBE_CACHE_HOME=/tmp/clashcube-cache
    case "$clashcube_storage_mode" in
        persistent) CLASHCUBE_CORE_HOME="$CLASHCUBE_HOME/core" ;;
        ram) CLASHCUBE_CORE_HOME=/tmp/clashcube-data/core ;;
        external)
            # Refuse to fill internal flash when a removable disk is absent.
            case "$clashcube_external_mount" in /mnt/*|/media/*) ;; *) echo 'Invalid ClashCube external mount' >&2; return 1;; esac
            awk -v path="$clashcube_external_mount" '$2 == path { found=1 } END { exit !found }' /proc/mounts || {
                echo 'ClashCube external storage is not mounted' >&2; return 1;
            }
            CLASHCUBE_CORE_HOME="$clashcube_external_mount/clashcube/core"
            ;;
        *) echo 'ClashCube storage_mode must be persistent, ram or external' >&2; return 1;;
    esac
    mkdir -p "$CLASHCUBE_HOME/core" "$CLASHCUBE_CORE_HOME" "$CLASHCUBE_CACHE_HOME"
    chmod 700 "$CLASHCUBE_HOME" "$CLASHCUBE_HOME/core" "$CLASHCUBE_CORE_HOME" "$CLASHCUBE_CACHE_HOME"
    # Back up configuration/identity, not redownloadable GEO/provider caches.
    # Generate this list so custom persistent data_dir values are covered too.
    mkdir -p /lib/upgrade/keep.d
    cat > /lib/upgrade/keep.d/clashcube-data <<EOF
$CLASHCUBE_HOME/settings.json
$CLASHCUBE_HOME/profiles/
$CLASHCUBE_HOME/modules.json
$CLASHCUBE_HOME/rules.json
$CLASHCUBE_HOME/router.json
$CLASHCUBE_HOME/router-maintenance.json
$CLASHCUBE_HOME/geo-schedule.json
$CLASHCUBE_HOME/core-service.json
$CLASHCUBE_HOME/dns-restore.json
$CLASHCUBE_HOME/core/runtime.yaml
$CLASHCUBE_HOME/core/tailscale/
EOF
    if [ "$CLASHCUBE_CORE_HOME" != "$CLASHCUBE_HOME/core" ]; then
        # Managed Tailscale identities survive RAM-cache loss and storage-mode
        # changes. Do not move a live identity; switch modes with services off.
        mkdir -p "$CLASHCUBE_HOME/core/tailscale"
        if [ ! -e "$CLASHCUBE_CORE_HOME/tailscale" ]; then
            ln -s "$CLASHCUBE_HOME/core/tailscale" "$CLASHCUBE_CORE_HOME/tailscale"
        fi
    fi
    SAFE_PATHS="$CLASHCUBE_HOME/core"
    export CLASHCUBE_HOME CLASHCUBE_CORE_HOME CLASHCUBE_CACHE_HOME SAFE_PATHS
}
