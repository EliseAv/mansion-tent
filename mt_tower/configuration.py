import functools
import json
import urllib.parse
from pathlib import Path


class Configuration:
    discord_token = ""
    discord_channel = -1
    s3_prefix = "s3://"
    ec2_ami_query = "amzn2-ami-kernel-*-hvm-*-x86_64-gp2"
    ec2_instance_type = "c5a.large"
    ec2_security_group = ""
    ec2_key_pair = ""
    ec2_iam_role = ""
    r53_zone_id = ""
    r53_record_name = "factorio"

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
