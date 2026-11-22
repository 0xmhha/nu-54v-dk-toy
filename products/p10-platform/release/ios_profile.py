"""Validate independent P02 practice-profile upgrade and rollback evidence."""

from pathlib import Path

from evidence import require

FORMAT = "nu54-ios-profile-v1"
CONTRACT = "practice-resume-v1"
STEPS = ("sourceWrite", "upgradeReadAndWrite", "rollbackRead")
TTL = 7 * 86400


def check(wrapped, source, target, policy, now, excluded=()):
    from fleet import key_identity, signature, text_hash

    require(isinstance(wrapped, dict) and set(wrapped) == {"claim", "signer", "signature"},
            "iOS existing-profile hold: independent continuity evidence missing")
    claim = wrapped["claim"]
    expected = {"format": FORMAT, "contract": CONTRACT, "source": source["payload"], "target": target["payload"],
                "package": target["bundleId"], "os": target["dimensions"]["os"], "identity": target.get("profileIdentity") }
    require(isinstance(expected["identity"], str) and source.get("profileIdentity") == expected["identity"] and
            source.get("bundleId") == target["bundleId"] and source["payload"] != target["payload"] and
            isinstance(claim, dict) and set(claim) == {*expected, "observedAt", "expiresAt", "steps"} and
            all(claim[key] == value for key, value in expected.items()),
            "iOS existing-profile hold: continuity evidence names a different transition")
    text_hash(expected["identity"])
    observed, expires = claim["observedAt"], claim["expiresAt"]
    require(type(observed) is int and type(expires) is int and observed <= now < expires <= observed + TTL,
            "iOS existing-profile hold: continuity evidence stale")
    grant = policy.get("iosProfileVerifiers", {}).get(wrapped["signer"])
    require(isinstance(grant, dict) and wrapped["signer"] not in excluded and grant.get("revoked") is not True and
            target["bundleId"] in grant.get("packages", ()) and target["dimensions"]["os"] in grant.get("os", ()),
            "iOS existing-profile hold: continuity verifier not enrolled")
    public = Path(grant["key"])
    require(key_identity(public) == wrapped["signer"], "iOS existing-profile hold: continuity verifier key differs")
    signature(claim, wrapped["signature"], public)
    steps = claim["steps"]
    require(isinstance(steps, dict) and set(steps) == set(STEPS), "iOS existing-profile hold: incomplete continuity")
    for step in STEPS:
        result = steps[step]
        require(isinstance(result, dict) and set(result) == {"passed", "result"} and result["passed"] is True,
                "iOS existing-profile hold: continuity check failed")
        text_hash(result["result"])
    return expires, wrapped["signer"]
