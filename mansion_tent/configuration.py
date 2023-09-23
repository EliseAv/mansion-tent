import json
from pathlib import Path


class Configuration:
    discord_webhook = ""
    autoquit_start_wait_minutes = 10
    autoquit_drain_wait_minutes = 1
    ec2_ami_query = "amzn2-ami-kernel-*-hvm-*-x86_64-gp3"
    ec2_instance_type = "c5a.large"
    ec2_security_group = ""
    ec2_key_pair = ""
    ec2_iam_role = ""

    def __init__(self):
        payload = json.load(Path(__file__).resolve().with_suffix(".json").open())
        for key, value in payload.items():
            if hasattr(Configuration, key):
                setattr(self, key, value)


cfg = Configuration()
