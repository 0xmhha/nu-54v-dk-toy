"""Check the independently observed Android retained practice contract."""

from pathlib import Path

from evidence import require

FORMAT = "nu54-android-profile-v1"
CONTRACT = "practice-resume-v1"
STEPS = ("sourceWrite", "upgradeReadAndWrite", "rollbackRead")
TTL = 7 * 86400


def check(wrapped, source, target, policy, now, excluded):
    from fleet import key_identity, signature, text_hash

    prefix = "Android existing-profile hold"
    require(source.get("package")==target.get("package") and
            source.get("certificate")==target.get("certificate") and
            source.get("versionCode",0)<target.get("versionCode",0) and
            target.get("rollbackDataPolicy")=="retain" and source["payload"]!=target["payload"],
            f"{prefix}: incompatible APK identity, version or data policy")
    require(isinstance(wrapped,dict) and set(wrapped)=={"claim","signer","signature"},
            f"{prefix}: independent continuity evidence missing")
    claim = wrapped["claim"]
    expected={"format":FORMAT,"contract":CONTRACT,"source":source["payload"],"target":target["payload"],
              "package":target["package"],"os":target["dimensions"]["os"],"certificate":target["certificate"],
              "rollbackDataPolicy":"retain"}
    require(isinstance(claim,dict) and set(claim)=={*expected,"observedAt","expiresAt","steps"} and
            all(claim[key]==value for key,value in expected.items()),
            f"{prefix}: continuity evidence names a different transition")
    observed, expires = claim["observedAt"], claim["expiresAt"]
    require(type(observed) is int and type(expires) is int and observed<=now<expires<=observed+TTL,
            f"{prefix}: continuity evidence stale")
    verifier=text_hash(wrapped["signer"])
    grant=policy.get("androidProfileVerifiers",{}).get(verifier)
    require(isinstance(grant,dict) and verifier not in excluded and grant.get("revoked") is not True and
            target["package"] in grant.get("packages",()) and target["dimensions"]["os"] in grant.get("os",()),
            f"{prefix}: continuity verifier not enrolled")
    public=Path(grant["key"])
    require(key_identity(public)==verifier,f"{prefix}: continuity verifier key differs")
    signature(claim,wrapped["signature"],public)
    steps=claim["steps"]
    require(isinstance(steps,dict) and set(steps)==set(STEPS),f"{prefix}: incomplete continuity")
    for step in STEPS:
        result=steps[step]
        require(isinstance(result,dict) and set(result)=={"passed","result"} and result["passed"] is True,
                f"{prefix}: continuity check failed")
        text_hash(result["result"])
    return expires,verifier


PROTECTED_FORMAT = "nu54-android-protected-v1"
PROTECTED_CONTRACT = "c1-retained-local-v1"
PROTECTED_STEPS = ("sourceUnresolved","upgradePending","rollbackPending")
PROTECTED_SCOPE = "protected-profile-local"


def check_protected(wrapped, source, target, policy, now, excluded):
    from fleet import key_identity, signature, text_hash

    prefix="Android protected profile hold"
    require(source.get("package")==target.get("package") and source.get("certificate")==target.get("certificate")
            and source.get("versionCode",0)<target.get("versionCode",0) and source["payload"]!=target["payload"]
            and target.get("rollbackDataPolicy")=="retain" and target["dimensions"]["os"]=="android-api-34",
            f"{prefix}: incompatible APK or rollback policy")
    require(isinstance(wrapped,dict) and set(wrapped)=={"claim","signer","signature"},
            f"{prefix}: independent continuity evidence missing")
    claim=wrapped["claim"]
    expected={"format":PROTECTED_FORMAT,"contract":PROTECTED_CONTRACT,"source":source["payload"],"target":target["payload"],
              "package":target["package"],"os":target["dimensions"]["os"],"certificate":target["certificate"],
              "rollbackDataPolicy":"retain","scenario":"expired-unattempted-c1","authority":"synthetic-local"}
    require(isinstance(claim,dict) and set(claim)=={*expected,"observedAt","expiresAt","steps","rawHash","ownerScope","assurance"}
            and all(claim[key]==value for key,value in expected.items()),
            f"{prefix}: evidence names a different APK transition")
    require(claim["assurance"] in {"owner-hardware-available","owner-hardware-unavailable"},
            f"{prefix}: invalid hardware identity observation")
    text_hash(claim["rawHash"])
    text_hash(claim["ownerScope"])
    observed,expires=claim["observedAt"],claim["expiresAt"]
    require(type(observed) is int and type(expires) is int and observed<=now<expires<=observed+TTL,
            f"{prefix}: continuity evidence stale")
    verifier=text_hash(wrapped["signer"])
    grant=policy.get("androidProfileVerifiers",{}).get(verifier)
    require(isinstance(grant,dict) and verifier not in excluded and grant.get("revoked") is not True and
            target["package"] in grant.get("packages",()) and target["dimensions"]["os"] in grant.get("os",()) and
            PROTECTED_CONTRACT in grant.get("contracts",()),f"{prefix}: protected verifier not independently enrolled")
    public=Path(grant["key"])
    require(key_identity(public)==verifier,f"{prefix}: continuity verifier key differs")
    signature(claim,wrapped["signature"],public)
    steps=claim["steps"]
    require(isinstance(steps,dict) and set(steps)==set(PROTECTED_STEPS),f"{prefix}: incomplete continuity")
    for step in PROTECTED_STEPS:
        outcome=steps[step]
        require(isinstance(outcome,dict) and set(outcome)=={"passed","result"} and outcome["passed"] is True,
                f"{prefix}: security-state continuity not verified")
        text_hash(outcome["result"])
    return expires,verifier
