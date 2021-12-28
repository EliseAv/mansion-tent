import asyncio.subprocess
import logging
import os
import socket
import subprocess
import tarfile
import tempfile
import typing as t
from pathlib import Path

import aioboto3
import aiohttp
import botocore.exceptions

from mt_camp.configuration import cfg
from mt_camp.runner import Runner

log = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)


async def main():
    async with session.resource("s3") as s3, aiohttp.ClientSession() as http:
        bucket = await s3.Bucket(cfg.s3_bucket)
        async with Launcher(http, bucket) as launcher:
            await launcher.run()


class Launcher:
    def __init__(self, http: aiohttp.ClientSession, bucket):
        self.http = http
        self.bucket = bucket
        self.saves = Path("factorio", "saves").resolve()
        self.sending = asyncio.Lock()

    async def __aenter__(self):
        await asyncio.gather(self.download_game(), self.download_config(), self.download_save())
        return self

    async def download_game(self):
        if Path("factorio", "licenses.txt").exists():
            return  # already downloaded
        url = f"https://www.factorio.com/get-download/{cfg.factorio_version}/headless/linux64"
        with tempfile.TemporaryFile() as f:
            async with self.http.get(url) as response:
                async for chunk in response.content.iter_chunked(0x1000):
                    f.write(chunk)
            f.seek(0)  # rewind
            with tarfile.open(fileobj=f, mode="r:xz") as tar:
                await asyncio.get_running_loop().run_in_executor(None, tar.extractall)

    async def download_config(self):
        with tempfile.TemporaryFile() as f:
            try:
                await self.bucket.download_fileobj(f"{cfg.s3_key_prefix}config.tar.gz", f)
            except botocore.exceptions.ClientError:
                return  # not essential
            f.seek(0)  # rewind
            with tarfile.open(fileobj=f, mode="r:gz") as tar:
                await asyncio.get_running_loop().run_in_executor(None, tar.extractall)

    async def download_save(self):
        save = Path("factorio", "saves", "world.zip")
        if save.exists():
            return  # already downloaded
        save.parent.mkdir(parents=True, exist_ok=True)
        with save.open("wb") as f:
            await self.bucket.download_fileobj(f"{cfg.s3_key_prefix}world.zip", f)

    async def run(self):
        os.chdir("factorio")
        process = await asyncio.create_subprocess_exec(
            "bin/x64/factorio",
            "--start-server",
            "world",
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        await Runner(process, self).run()

    async def announce_my_ip_address(self):
        url = "http://169.254.169.254/latest/meta-data/public-ipv4"
        try:
            async with self.http.get(url) as response:
                result = await response.text()
        except aiohttp.ClientError:
            # https://stackoverflow.com/a/28950776/98029
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
                s.connect(("10.255.255.255", 1))  # doesn't even have to be reachable
                result = s.getsockname()[0]
        await self.chat(f"Server is ready at: `{result}` (`{cfg.public_hostname}`)")

    async def announce_players_change(self, players: t.Set[str], join: str = "", leave: str = ""):
        number = len(players)
        if join:
            person = f":star2: {join}"
        elif leave:
            person = f":comet: {leave}"
        else:
            person = ", ".join(sorted(players))
        await self.chat(f"[`{number:2}`] {person}")

    async def saving_finished(self):
        save = max(self.saves.glob("*.zip"), key=_get_mtime)
        async with self.sending:
            await self.bucket.upload_file(str(save), cfg.s3_key_prefix + save.name)
        log.info("Saved: %s", save)

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.chat("Server closed.")

        # make sure we will save before we terminate
        await asyncio.sleep(1)
        await self.sending.acquire()

        # terminate
        await asyncio.subprocess.create_subprocess_exec("/usr/bin/sudo", "/usr/sbin/poweroff")

    async def chat(self, message: str):
        payload = {"content": message}
        if not cfg.discord_webhook:
            log.info("Would have chatted: %s", payload)
            return
        async with self.http.post(cfg.discord_webhook, json=payload) as response:
            if response.status >= 300:
                log.warning("Discord said %s to %s", response.status, payload)


def _get_mtime(path: Path) -> float:
    return path.stat().st_mtime


session = aioboto3.Session()

if __name__ == "__main__":
    asyncio.run(main())
