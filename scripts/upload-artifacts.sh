#!/usr/bin/env bash

# Upload binaries/packages to github. Designed to be used from circleCI

[ -z "$GITHUB_GHR_TOKEN" ] && echo "Need to set $GITHUB_GHR_TOKEN for binary uploads" && exit 1;
wget https://github.com/tcnksm/ghr/releases/download/v0.5.3/ghr_v0.5.3_linux_amd64.zip
unzip ghr_v0.5.3_linux_amd64.zip
./ghr -t $GITHUB_GHR_TOKEN -u $CIRCLE_PROJECT_USERNAME -r $CIRCLE_PROJECT_REPONAME -delete $CIRCLE_TAG dist/
