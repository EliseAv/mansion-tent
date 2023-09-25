import json
from pathlib import Path


class Configuration:
    discord_webhook = ""
    autoquit_start_wait_minutes = 10
    autoquit_drain_wait_minutes = 1
    ec2_ami_query = "al2023-ami-2023.2.*-x86_64"
    ec2_instance_type = "c5a.large"
    ec2_security_group_id = ""
    ec2_key_pair_name = ""
    ec2_iam_role_name = ""
    ec2_subnet_id = ""

    def __init__(self):
        payload = json.load(Path(__file__).resolve().with_suffix(".json").open())
        for key, value in payload.items():
            if hasattr(Configuration, key):
                setattr(self, key, value)
            else:
                raise AttributeError(f"Unknown configuration key: {key}")


cfg = Configuration()


def main():
    import pprint

    pprint.pprint(vars(cfg))


if __name__ == "__main__":
    main()
