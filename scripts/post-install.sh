#!/bin/bash

DATA_DIR=/var/lib/nexus-server
LOG_DIR=/var/log/nexus-server
SCRIPT_DIR=/usr/lib/nexus-server/scripts

function install_systemd {
    cp -f $SCRIPT_DIR/nexus-server.service /lib/systemd/system/nexus-server.service
    systemctl enable nexus-server
}

id $USER &>/dev/null
if [[ $? -ne 0 ]]; then
    useradd --system -U -M $USER -s /bin/false -d $DATA_DIR
fi

chown -R -L $USER:$USER $DATA_DIR
chown -R -L $USER:$USER $LOG_DIR

# Distribution-specific logic
if [[ -f /etc/debian_version ]]; then
    # Debian/Ubuntu logic
    which systemctl &>/dev/null
    if [[ $? -eq 0 ]]; then
	install_systemd
    fi
fi
