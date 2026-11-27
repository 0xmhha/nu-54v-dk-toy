import { createHash, timingSafeEqual, X509Certificate } from "node:crypto";
import type { TLSSocket } from "node:tls";
import { V2_ALPN, V2_EXPORTER_LABEL, V2Link, v2PeerRole, type V2Journal, type V2Profile, type V2Role } from "./transport-v2.ts";

export function v2SpkiSha256(certificate: X509Certificate): Buffer {
  return createHash("sha256").update(certificate.publicKey.export({ type: "spki", format: "der" })).digest();
}

/** Call on the secure event of a TLS-1.3-only mutually verified socket configured with an explicit CA.
 * Native mobile builds use their TLS engine's equivalent verifier and exporter instead of node:tls. */
export function bindV2NodeTls(
  socket: TLSSocket, profile: V2Profile, local: V2Role, expectedPeer: Uint8Array, journal: V2Journal,
): V2Link {
  const alpn = V2_ALPN[profile];
  if (socket.getProtocol() !== "TLSv1.3" || !socket.authorized || socket.isSessionReused() || socket.alpnProtocol !== alpn) {
    throw new Error("TLS transport is not mutually verified for this v2 profile");
  }
  const certificate = socket.getPeerX509Certificate();
  if (!certificate || certificate.subjectAltName !== `URI:urn:nu54:role:${v2PeerRole(profile, local)}`) {
    throw new Error("TLS peer certificate has the wrong role");
  }
  const peer = v2SpkiSha256(certificate);
  if (expectedPeer.length !== peer.length || !timingSafeEqual(peer, expectedPeer)) throw new Error("TLS peer identity does not match the pin");
  const epoch = socket.exportKeyingMaterial(16, V2_EXPORTER_LABEL, Buffer.from(alpn, "ascii"));
  return new V2Link(profile, local, epoch, peer, journal);
}
