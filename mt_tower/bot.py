import asyncio
import logging

import aioboto3
from discord import Intents
from discord.ext.commands import Context, command, Bot, Command

from mt_tower.configuration import cfg

log = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s:  %(message)s")


class MyClient(Bot):
    def __init__(self):
        intents = Intents.none()
        intents.messages = True  # guild_messages|dm_messages
        super(MyClient, self).__init__("*", intents=intents)
        self.ec2 = aioboto3.Session().client("ec2")

    async def on_ready(self):
        self.ec2 = await self.ec2.__aenter__()
        log.info(f"Logged on as {self.user}")
        self.add_command(Command(self.play))

    async def play(self, ctx: Context):
        """ Starts the Factorio server for play! """
        working = "\U0001f3d7"  # construction site
        await ctx.message.add_reaction(working)
        try:
            await self.launch_instance()
            result = "\U0001f3ed"  # factory
        except Exception as e:
            log.error("Launch failed!", exc_info=e)
            result = "\U0001f525"  # fire
        await asyncio.gather(ctx.message.add_reaction(result), ctx.message.remove_reaction(working, ctx.me))

    async def launch_instance(self):
        await asyncio.sleep(1)
        log.info("Totally launched an instance on %s!", self.ec2)


if __name__ == "__main__":
    MyClient().run(cfg.discord_token)
