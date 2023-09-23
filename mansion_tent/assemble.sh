#!/bin/bash -xe

yum install -y python3-pip
pip3 install aiohttp
export PYTHONPATH=$HOME

screen -dmS camp python3 mt_camp/launch.py
