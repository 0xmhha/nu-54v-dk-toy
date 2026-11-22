"""Local-only signed enrollment for the installed W28 network chooser."""
import argparse
import base64
from pathlib import Path
import signal
import ssl
import tempfile
import time

from wallet_connect_spend import fixture
from run_wallet_chain_nodes import lab

EVM = {
    "8283": "2bdf79b3d3cc49f9e6638ff81f3bb85065c79945a8fe4556cd0ff47bbfc02490",
    "11155111": "25a5cc106eea7138acab33231d7160d69cb777ee0c2c553fcddf5138993e6dd9",
    "84532": "0dcc9e089e30b90ddfc55be9a37dd15bc551aeee999d2e2b51414c54eaf934e4",
}


def main(options):
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(options.cert, options.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(options.cert.read_text())).decode()
    profiles = [{"chain": index, "profile": profile, "anchor":lab.COMMISSIONED[index],
                 "rpc":"https://wallet.localhost:18449/"+route}
                for index,(profile,route) in enumerate((
                    ("bitcoin-signet","bitcoin"),("solana-devnet","solana"),("xrpl-testnet","xrpl")),1)
                if not options.omit_xrp or index!=3]
    evm = [{"chain":chain,"genesis":"0x"+genesis,"rpc":"https://127.0.0.1:18449/rpc-native",
            "feeLimit":"100000000","assets":[{"address":"0x"+"00"*20,"symbol":"ETH","decimals":0}]}
           for chain,genesis in EVM.items()]

    def refuse(*_):
        raise ValueError("Profile chooser performs no RPC egress")

    with tempfile.TemporaryDirectory(prefix="w28-chooser.") as directory:
        server, worker, _, _ = fixture(Path(directory),refuse,"0x"+EVM["11155111"],None,None,tls,{
            "cert":cert,"relay":"wss://127.0.0.1:18449","evm_profiles":evm,"commissioned_profiles":profiles,
            "case":{"missingXrp":options.omit_xrp}})
        print("W28 chooser fixture ready",flush=True)
        try:
            while True:
                time.sleep(1)
        finally:
            server.shutdown()
            server.server_close()
            worker.join()


def interrupt(_signal,_frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cert", type=Path)
    parser.add_argument("key", type=Path)
    parser.add_argument("--omit-xrp",action="store_true")
    signal.signal(signal.SIGTERM,interrupt)
    try:
        main(parser.parse_args())
    except KeyboardInterrupt:
        pass
