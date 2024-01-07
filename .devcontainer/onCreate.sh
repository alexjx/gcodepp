#!/bin/bash
set -ex

if [ ! -f /usr/local/bin/git-mreq ]; then
    sudo curl -o /usr/local/bin/git-mreq \
        --header "PRIVATE-TOKEN: WDgVsGaSPfmXyvmaYqGi" \
        "https://git.fastonetech.com:8443/api/v4/projects/336/repository/files/git-mreq/raw"

    sudo chmod a+x /usr/local/bin/git-mreq
fi
