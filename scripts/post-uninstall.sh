#!/bin/bash

function disable_systemd {
    systemctl disable nexus-server
    rm -f /lib/systemd/system/nexus-server.service
}

if [[ -f /etc/debian_version ]] && [[ "$1" != "upgrade" ]]; then
    # Debian/Ubuntu logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
        disable_systemd
    fi
fi
