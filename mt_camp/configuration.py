import functools
import json
import urllib.parse
from pathlib import Path


class Configuration:
    factorio_version = "stable"  # e.g. 1.1.48, stable, latest
    s3_prefix = "s3://"
    discord_webhook = ""
    start_wait_seconds = 60
    drain_wait_seconds = 10

    def __init__(self):
        payload = json.load(Path(__file__).resolve().with_suffix(".json").open())
        for key, value in payload.items():
            if hasattr(Configuration, key):
                setattr(self, key, value)

    @property
    @functools.lru_cache()
    def s3_bucket(self) -> str:
        return urllib.parse.urlparse(self.s3_prefix).netloc

    @property
    @functools.lru_cache()
    def s3_key_prefix(self) -> str:
        return urllib.parse.urlparse(self.s3_prefix).path.lstrip("/")


cfg = Configuration()
