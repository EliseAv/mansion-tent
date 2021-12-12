import asyncio
import logging
import operator
import tarfile
import tempfile
import typing
from pathlib import Path

import aioboto3

from mt_tower.configuration import cfg
from mt_tower.reaction import Reaction

log = logging.getLogger(__name__)


class Dispatcher:
    def __init__(self):
        session = aioboto3.Session()
        self.client = session.client("ec2")
        self.resource = session.resource("ec2")
        self.s3 = session.resource("s3")
        self.bucket = None

    async def __aenter__(self):
        self.client = await self.client.__aenter__()
        self.resource = await self.resource.__aenter__()
        self.s3 = await self.s3.__aenter__()
        self.bucket = await self.s3.Bucket(cfg.s3_bucket)
        return self

    async def __aexit__(self, *_):
        await self.client.__aexit__(*_)
        await self.resource.__aexit__(*_)

    async def launch(self) -> Reaction:
        if await self.is_it_already_running():
            return Reaction.stop

        ami, userdata = await asyncio.gather(self.find_ami(), self.build_userdata())
        args = {
            "ImageId": ami,
            "InstanceType": cfg.ec2_instance_type,
            "MinCount": 1,
            "MaxCount": 1,
            "SecurityGroupIds": [cfg.ec2_security_group],
            "UserData": userdata,
            "InstanceInitiatedShutdownBehavior": "terminate",
            "IamInstanceProfile": {"Name": cfg.ec2_iam_role},
            "DryRun": False,
            "TagSpecifications": [
                {
                    "ResourceType": r,
                    "Tags": [{"Key": "Name", "Value": "Auto-Factorio"}],
                }
                for r in ("instance", "volume")
            ],
        }
        if cfg.ec2_key_pair:
            args["KeyName"] = cfg.ec2_key_pair
        result = await self.client.run_instances(**args)
        log.debug(result)

        return Reaction.built

    @staticmethod
    async def is_it_already_running() -> bool:  # TODO
        return False

    async def find_ami(self) -> str:
        query = [{"Name": "name", "Values": [cfg.ec2_ami_query]}]
        result = await self.client.describe_images(Filters=query, Owners=["amazon"])
        images = result["Images"]
        best = max(images, key=operator.itemgetter("CreationDate"))
        log.info("Chose AMI: %s", best["Description"])
        return best["ImageId"]

    async def build_userdata(self) -> bytes:
        offset = asyncio.get_running_loop().run_in_executor
        with tempfile.TemporaryFile() as f:
            await offset(None, build_tar_package, f)
            f.seek(0)  # rewind
            key = cfg.s3_key_prefix + "camp.tar.gz"
            await self.bucket.upload_fileobj(f, key)
        lines = [
            "#!/bin/bash",
            f"aws s3 cp s3://{cfg.s3_bucket}/{key} - | tar xzC /home/ec2-user",
            "chown -R ec2-user:ec2-user /home/ec2-user",
            "sudo -iu ec2-user /bin/bash mt_camp/assemble.sh",
        ]
        return b"\n".join(s.encode() for s in lines)


def build_tar_package(f: typing.BinaryIO):
    with tarfile.open(fileobj=f, mode="w:gz") as tar:
        camp = Path(__file__).resolve().parent.parent.joinpath("mt_camp")
        for path in camp.iterdir():
            if path.is_file():
                info = tarfile.TarInfo("mt_camp/" + path.name)
                info.size = path.stat().st_size
                tar.addfile(info, path.open("rb"))
