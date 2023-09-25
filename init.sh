#!/bin/bash -xe

# https://www.factorio.com/download
curl -sL https://www.factorio.com/get-download/latest/headless/linux64 | tar xvJ
mkdir -p factorio/saves factorio/mods

poetry install
