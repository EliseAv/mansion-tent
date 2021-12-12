#!/usr/bin/env bash

pip3 install -r mt_camp/requirements.txt
export PYTHONPATH=$HOME

screen -dmS camp python3 mt_camp/launch.py
