"""QA-only CONNECT front door for the exact commissioned wallet hostname."""
import argparse
import select
import socket
from socketserver import BaseRequestHandler, ThreadingTCPServer


class ExactWalletTunnel(BaseRequestHandler):
    def handle(self):
        self.request.settimeout(5)
        header = bytearray()
        while not header.endswith(b"\r\n\r\n"):
            data = self.request.recv(1)
            if not data or len(header) >= 4096:
                return
            header.extend(data)
        request = bytes(header).split(b"\r\n", 1)[0]
        if request not in (b"CONNECT wallet.localhost:18449 HTTP/1.1", b"CONNECT wallet.localhost:18449 HTTP/1.0"):
            self.request.sendall(b"HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n")
            return
        print("Accepted isolated wallet TLS tunnel",flush=True)
        with socket.create_connection(("127.0.0.1", self.server.wallet_port), timeout=5) as upstream:
            self.request.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
            peers = {self.request: upstream, upstream: self.request}
            while True:
                ready, _, _ = select.select(list(peers), [], [], 12)
                if not ready:
                    return
                for sock in ready:
                    packet = sock.recv(16384)
                    if not packet:
                        return
                    peers[sock].sendall(packet)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18452)
    parser.add_argument("--wallet-port", type=int, default=18451)
    args = parser.parse_args()
    with ThreadingTCPServer(("127.0.0.1", args.port), ExactWalletTunnel) as server:
        server.daemon_threads = True
        server.wallet_port = args.wallet_port
        server.serve_forever()
