import asyncio
import logging
import operator

import aioboto3
import aiohttp

from .configuration import cfg

log = logging.getLogger(__name__)


async def dispatch():
    session = aioboto3.Session()
    async with session.client("ec2") as ec2:
        ami, userdata = await asyncio.gather(find_ami(ec2), build_userdata())
        instance_id = await run_instance(ec2, ami, userdata)
    log.info("Launched instance %s", instance_id)


async def find_ami(ec2) -> str:
    query = [{"Name": "name", "Values": [cfg.ec2_ami_query]}]
    result = await ec2.describe_images(Filters=query, Owners=["amazon"])
    images = result["Images"]
    if not images:
        raise ValueError(f"No AMI found. Query was: {cfg.ec2_ami_query}")
    best = max(images, key=operator.itemgetter("CreationDate"))
    log.info("Found AMI: %s", best["Description"])
    return best["ImageId"]


async def build_userdata() -> bytes:
    url = "http://169.254.169.254/latest/meta-data/local-ipv4"
    async with aiohttp.ClientSession() as session:
        async with session.get(url) as response:
            dispatcher_ip = await response.text()
    lines = [
        "#!/bin/bash",
        "mkdir -p /opt/mansion-tent",
        f"mount {dispatcher_ip}:/opt/mansion-tent /opt/mansion-tent",
        "yum install -y python3.11-pip",
        "pip3.11 install poetry",
        "chown -R ec2-user:ec2-user /opt/mansion-tent",
        "sudo -iu ec2-user poetry -C /opt/mansion-tent install",
        "sudo -iu ec2-user screen -dmS factorio poetry -C /opt/mansion-tent run launch",
    ]
    return b"\n".join(s.encode() for s in lines)


async def run_instance(ec2, ami: str, userdata: bytes) -> str:
    args = {
        "ImageId": ami,
        "InstanceType": cfg.ec2_instance_type,
        "MinCount": 1,
        "MaxCount": 1,
        "SecurityGroupIds": [cfg.ec2_security_group_id],
        "UserData": userdata,
        "InstanceInitiatedShutdownBehavior": "terminate",
        "IamInstanceProfile": {"Name": cfg.ec2_iam_role_name},
        "DryRun": False,
        "TagSpecifications": [
            {
                "ResourceType": r,
                "Tags": [{"Key": "Name", "Value": "Auto-Factorio"}],
            }
            for r in ("instance", "volume")
        ],
        "MetadataOptions": {"HttpTokens": "optional"},
    }
    if cfg.ec2_subnet_id:
        args["SubnetId"] = cfg.ec2_subnet_id
    if cfg.ec2_key_pair_name:
        args["KeyName"] = cfg.ec2_key_pair_name
    result = await ec2.run_instances(**args)
    return result["Instances"][0]["InstanceId"]


def main():
    asyncio.run(dispatch())


if __name__ == "__main__":
    main()
