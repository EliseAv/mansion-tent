import asyncio.subprocess
import logging
import os
from pathlib import Path
import socket
import subprocess
import typing as t

import aiohttp

from .configuration import cfg
from .runner import Runner

log = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)


async def main():
    async with Launcher() as launcher:
        await launcher.run()


class Launcher:
    def __init__(self):
        self.http = aiohttp.ClientSession()

    async def __aenter__(self):
        self.http = await self.http.__aenter__()
        return self

    async def run(self):
        os.chdir("factorio")
        args = ["bin/x64/factorio", "--start-server", "saves/world.zip"]
        if Path("server-settings.json").exists():
            args.extend(("--server-settings", "server-settings.json"))
        process = await asyncio.create_subprocess_exec(
            *args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
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
                s.connect(("143.54.1.1", 9))  # doesn't even have to be reachable
                result = s.getsockname()[0]
        await self.chat(f"Server is ready at: `{result}`")

    async def announce_players_change(self, players: t.Set[str], join: str = "", leave: str = ""):
        number = len(players)
        if join:
            person = f":star2: {join}"
        elif leave:
            person = f":comet: {leave}"
        else:
            person = ", ".join(sorted(players))
        await self.chat(f"[`{number:2}`] {person}")

    async def __aexit__(self, *args):
        await self.chat("Server closed.")
        await self.http.__aexit__(*args)

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


if __name__ == "__main__":
    asyncio.run(main())
