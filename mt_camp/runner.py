import asyncio
import re
import sys
import typing as t
from time import time_ns

from mt_camp.configuration import cfg

if t.TYPE_CHECKING:
    from mt_camp.launch import Launcher

SECOND = 1000000000
START_WAIT_NS = int(cfg.start_wait_seconds * SECOND)
DRAIN_WAIT_NS = int(cfg.drain_wait_seconds * SECOND)


class MessageMatcher(dict):
    def __call__(self, pattern: re.Pattern):
        def wrapper(function):
            self[pattern] = function
            return function

        return wrapper

    def dispatch(self, obj, line: str):
        for pattern, function in self.items():
            match = pattern.match(line)
            if match:
                asyncio.create_task(function(obj, match))
                return


class Runner:
    def __init__(self, process: asyncio.subprocess.Process, launcher: "Launcher"):
        self.process = process
        self.launcher = launcher
        self.players: t.Set[str] = set()
        self.exit_at = START_WAIT_NS + time_ns()
        self.watcher = asyncio.Event()

    async def run(self):
        tasks = [
            asyncio.create_task(coroutine)
            for coroutine in [
                self.handle_input(self.process.stdin, sys.stdin),
                self.handle_output(self.process.stdout, sys.stdout, self.matcher),
                self.handle_output(self.process.stderr, sys.stderr),
                self.watch_number_of_players(),
            ]
        ]

        await self.process.wait()

        for task in tasks:
            if not task.done():
                task.cancel()

    @staticmethod
    async def handle_input(writer: asyncio.StreamWriter, sync_reader: t.TextIO):
        # https://stackoverflow.com/a/64317899/98029
        loop = asyncio.get_running_loop()
        reader = asyncio.StreamReader()
        protocol = asyncio.StreamReaderProtocol(reader)
        await loop.connect_read_pipe(lambda: protocol, sync_reader)

        line = await reader.readline()
        while line:
            writer.write(line)
            line = await reader.readline()

    async def handle_output(self, reader: asyncio.StreamReader, sync_writer: t.TextIO, matcher=None):
        # https://stackoverflow.com/a/64317899/98029
        loop = asyncio.get_running_loop()
        transport, protocol = await loop.connect_write_pipe(asyncio.streams.FlowControlMixin, sync_writer)
        writer = asyncio.StreamWriter(transport, protocol, reader, loop)

        if not matcher:
            matcher = MessageMatcher()

        line = await reader.readline()
        while line:
            writer.write(line)
            matcher.dispatch(self, line.decode())
            line = await reader.readline()

    async def watch_number_of_players(self):
        exit_in = self.exit_at - time_ns()
        while exit_in > 0:
            await asyncio.sleep(exit_in / SECOND)

            # If there are players, sleep until there aren't.
            if self.players:
                self.watcher.clear()
                await self.watcher.wait()

            # Looks like the server is empty! Time for a countdown.
            exit_in = self.exit_at - time_ns()

        print("No activity! Exiting!")
        self.process.stdin.write(b"/quit\n")

    matcher = MessageMatcher()

    @matcher(
        re.compile(
            r"^\s*\d+\.\d+ Info ServerMultiplayerManager\.cpp:\d+: "
            r"updateTick\(\d+\) changing state from\(CreatingGame\) to\(InGame\)$"
        )
    )
    async def __event_launched(self, _: re.Match):
        await self.launcher.announce_my_ip_address()

    @matcher(re.compile(r"^....-..-.. ..:..:.. \[JOIN] (.+) joined the game$"))
    async def __event_joined(self, match: re.Match):
        name = match.group(1)
        self.players.add(name)
        await self.launcher.announce_players_change(self.players, join=name)

    @matcher(re.compile(r"^....-..-.. ..:..:.. \[LEAVE] (.+) left the game$"))
    async def __event_left(self, match: re.Match):
        name = match.group(1)
        self.players.discard(name)
        await self.launcher.announce_players_change(self.players, leave=name)
        if not self.players:
            self.exit_at = DRAIN_WAIT_NS + time_ns()
            self.watcher.set()

    @matcher(re.compile(r"^\s*\d+\.\d+ Info AppManagerStates\.cpp:\d+: Saving finished$"))
    async def __event_saved(self, _: re.Match):
        await self.launcher.saving_finished()
