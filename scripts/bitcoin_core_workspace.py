"""Own a disconnected foreground Bitcoin Core and its private data directory."""

from contextlib import contextmanager
import socket
import subprocess
import time

from integration_workspace import OwnedProcess, cleanup_stack, run_command, scoped_workspace


class BitcoinCore:
    def __init__(self, tools, directory, chain, port):
        self.tools = tools
        self.directory = directory
        self.port = port
        self.cookie = directory / chain / ".cookie"
        self.options = [f"-{chain}", f"-datadir={directory}", f"-rpcport={port}"]
        self.process = OwnedProcess()

    def start(self):
        child = self.process.start([
            self.tools / "bitcoind", *self.options, "-daemon=0", "-daemonwait=0",
            "-networkactive=0", "-listen=0", "-dnsseed=0", "-rpcbind=127.0.0.1",
        ], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL)
        deadline = time.monotonic() + 30
        while not self.cookie.is_file():
            if child.poll() is not None:
                raise RuntimeError(f"Owned Bitcoin Core exited during startup ({child.returncode})")
            if time.monotonic() >= deadline:
                raise TimeoutError("Owned Bitcoin Core did not create its RPC cookie")
            time.sleep(0.05)
        self.cli("-rpcwait", "-rpcwaittimeout=30", "getblockchaininfo",
                 timeout=max(1, deadline - time.monotonic()))
        if child.poll() is not None:
            raise RuntimeError(f"Owned Bitcoin Core exited during startup ({child.returncode})")

    def cli(self, *args, timeout=30):
        return run_command([
            self.tools / "bitcoin-cli", *self.options, "-rpcconnect=127.0.0.1",
            f"-rpccookiefile={self.cookie}", *args,
        ], check=True, stdout=subprocess.PIPE, text=True, timeout=timeout).stdout

    def stop(self):
        self.process.stop()


@contextmanager
def bitcoin_core_workspace(tools, prefix, chain):
    try:
        with scoped_workspace(prefix) as directory, cleanup_stack() as cleanup:
            with socket.socket() as local:
                local.bind(("127.0.0.1", 0))
                port = local.getsockname()[1]
            core = BitcoinCore(tools, directory, chain, port)
            cleanup.callback(core.stop)
            yield core
    except KeyboardInterrupt:
        raise SystemExit(130) from None
