"""Verify signed Ed25519 release images using the NCS MCUboot image parser."""

from pathlib import Path
import sys


def main() -> int:
    imgtool, public, *images = sys.argv[1:]
    sys.path.insert(0, str(Path(imgtool).parent))
    from imgtool import image, keys

    key = keys.load(public)
    if type(key) is not keys.Ed25519Public:
        sys.exit("Release verification requires a public Ed25519 key")
    # NCS 3.4.1 lists the private Ed25519 wrapper only. The public wrapper
    # implements the same verify_digest operation and needs the same hash policy.
    image.ALLOWED_KEY_SHA.setdefault(keys.Ed25519Public, image.ALLOWED_KEY_SHA[keys.Ed25519])
    for artifact in images:
        result, version, _, _ = image.Image.verify(artifact, key)
        if result != image.VerifyResult.OK:
            sys.exit(f"Release image verification failed ({result.name}): {artifact}")
        print(f"Verified MCUboot signature and image version {version}: {artifact}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
