import json
from pathlib import Path


class Configuration:
    discord_token = ""
    discord_channel = -1

    def __init__(self):
        payload = json.load(Path(__file__).resolve().with_suffix(".json").open())
        for key, value in payload.items():
            if hasattr(Configuration, key):
                setattr(self, key, value)


cfg = Configuration()
