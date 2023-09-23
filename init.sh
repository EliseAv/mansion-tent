#!/bin/bash -xe

# https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html
curl -so dispatcher-ip.txt http://169.254.169.254/latest/meta-data/public-ipv4

# https://www.factorio.com/download
curl -sL https://www.factorio.com/get-download/latest/headless/linux64 | tar xvJ

pushd factorio
if ! [ -d saves ]; then
    mkdir saves mods
    echo 'Press Enter to create a new vanilla world, or'
    echo 'Ctrl+C to exit so you can configure existing setup'
    echo 'to factorio/saves/world.zip and factorio/mods'
    read
    bin/x64/factorio --create-world saves/world.zip
fi
popd
