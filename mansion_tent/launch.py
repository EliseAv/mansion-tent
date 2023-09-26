import asyncio
import logging
import os
import subprocess
import typing as t
from pathlib import Path

import aiohttp

from .configuration import cfg
from .runner import Runner

log = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)


class Launcher:
    @classmethod
    async def main(cls):
        async with aiohttp.ClientSession() as http, cls(http) as launcher:
            await launcher.run()

    def __init__(self, http: aiohttp.ClientSession):
        self.http = http

    async def __aenter__(self):
        return self

    async def run(self):
        base_path = Path(__file__).resolve().parent.parent.joinpath("factorio")
        os.chdir(base_path)
        args = ["bin/x64/factorio", "--start-server", "saves/world.zip"]
        if Path("server-settings.json").exists():
            args.extend(("--server-settings", "server-settings.json"))
        process = await asyncio.create_subprocess_exec(
            *args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
        )
        await Runner(process, self).run()

    async def announce_my_ip_address(self):
        url = "http://169.254.169.254/latest/meta-data/public-ipv4"
        async with self.http.get(url) as response:
            response.raise_for_status()
            result = await response.text()
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

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        # make sure instance termination happens
        arguments = ["/usr/bin/sudo", "/usr/sbin/shutdown"]  # 1 minute delay
        asyncio.create_task(asyncio.create_subprocess_exec(*arguments))

        # announce
        if exc_type:
            try:
                await self.chat(f"Server crashed: `{exc_type.__name__}: {exc_val}`")
            except aiohttp.ClientError:
                pass
        else:
            await self.chat("Server shut down")

    async def chat(self, message: str):
        payload = {"content": message}
        if not cfg.discord_webhook:
            log.info("Would have chatted: %s", payload)
            return

        async with self.http.post(cfg.discord_webhook, json=payload) as response:
            if response.status >= 300:
                log.warning("Discord said %s to %s", response.status, payload)


def main():
    asyncio.run(Launcher.main())


if __name__ == "__main__":
    main()
