import asyncio
import logging

from discord import Intents
from discord.ext.commands import Context, Bot, Command

from mt_tower.configuration import cfg
from mt_tower.dispatch import Dispatcher
from mt_tower.reaction import Reaction

log = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s:  %(message)s")


class MyClient(Bot):
    def __init__(self):
        intents = Intents.none()
        intents.messages = True  # guild_messages|dm_messages
        super(MyClient, self).__init__("*", intents=intents)
        self.dispatcher = Dispatcher()
        self.launching = False

    async def on_ready(self):
        self.dispatcher = await self.dispatcher.__aenter__()
        log.info(f"Logged on as {self.user}")
        self.add_command(Command(self.play))

    async def play(self, ctx: Context):
        """ Starts the Factorio server for play! """
        if self.launching:
            log.warning("Already launching.")
            await ctx.message.add_reaction(Reaction.stop.value)
            return

        self.launching = True
        await ctx.message.add_reaction(Reaction.working.value)
        try:
            result = await self.dispatcher.launch()
        except Exception as e:
            log.error("Launch failed!", exc_info=e)
            result = Reaction.error
        await asyncio.gather(
            ctx.message.add_reaction(result.value),
            ctx.message.remove_reaction(Reaction.working.value, ctx.me),
        )
        self.launching = False


if __name__ == "__main__":
    MyClient().run(cfg.discord_token)
