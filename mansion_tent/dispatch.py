import asyncio
import logging
import operator
import tarfile
import tempfile
import typing
from pathlib import Path

import aioboto3

from .configuration import cfg

log = logging.getLogger(__name__)


class Dispatcher:
    def __init__(self):
        session = aioboto3.Session()
        self.ec2 = session.client("ec2")
        self.bucket = None

    async def __aenter__(self):
        self.ec2 = await self.ec2.__aenter__()
        return self

    async def __aexit__(self, *args):
        await self.ec2.__aexit__(*args)

    async def launch(self):
        ami, userdata = await asyncio.gather(self.find_ami(), self.build_userdata())
        instance_id = await self.run_instance(ami, userdata)
        log.info("Launched instance %s", instance_id)
        await self.update_dns(instance_id)
        log.info("DNS updated. Launch finished.")

    async def find_ami(self) -> str:
        query = [{"Name": "name", "Values": [cfg.ec2_ami_query]}]
        result = await self.ec2.describe_images(Filters=query, Owners=["amazon"])
        images = result["Images"]
        best = max(images, key=operator.itemgetter("CreationDate"))
        log.info("Found AMI: %s", best["Description"])
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

    async def run_instance(self, ami: str, userdata: bytes) -> str:
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
        result = await self.ec2.run_instances(**args)
        return result["Instances"][0]["InstanceId"]

    async def update_dns(self, instance_id: str):
        if not cfg.r53_zone_id:
            log.info("Route 53 disabled by configuration")
            return
        ip_address, zone = await asyncio.gather(
            self.get_ip_address(instance_id),
            self.r53.get_hosted_zone(Id=cfg.r53_zone_id),
        )
        if not ip_address:
            log.info("IP address unknown")
            return

        domain = zone["HostedZone"]["Name"]
        fqdn = f"{cfg.r53_record_name}.{domain}"
        resource_record_set = {
            "Name": fqdn,
            "Type": "A",
            "TTL": 60,
            "ResourceRecords": [{"Value": ip_address}],
        }
        await self.r53.change_resource_record_sets(
            HostedZoneId=cfg.r53_zone_id,
            ChangeBatch={
                "Comment": f"{ip_address} {fqdn}",
                "Changes": [{"Action": "UPSERT", "ResourceRecordSet": resource_record_set}],
            },
        )

    async def get_ip_address(self, instance_id: str) -> str:
        wait = 1
        for i in range(10):
            result = await self.ec2.describe_instances(InstanceIds=[instance_id])
            ip = result["Reservations"][0]["Instances"][0].get("PublicIpAddress")
            if ip:
                return ip
            log.info("IP address not available yet, retrying in %fs...", wait)
            await asyncio.sleep(wait)
            wait *= 1.618
        return ""  # unavailable


def build_tar_package(f: typing.BinaryIO):
    with tarfile.open(fileobj=f, mode="w:gz") as tar:
        camp = Path(__file__).resolve().parent.parent.joinpath("mt_camp")
        for path in camp.iterdir():
            if path.is_file():
                info = tarfile.TarInfo("mt_camp/" + path.name)
                info.size = path.stat().st_size
                tar.addfile(info, path.open("rb"))
