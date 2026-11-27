"""Reconcile only the simulator IDs owned by the installed café run."""

import json
import re
import subprocess
import time

SIMULATOR_ID = re.compile(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}", re.IGNORECASE)


def simulator_inventory(simctl):
    groups = json.loads(simctl("list", "devices", "--json", check=True).stdout)["devices"]
    if not isinstance(groups, dict) or not groups:
        raise ValueError("missing simulator inventory")
    devices_by_id = {}
    for devices in groups.values():
        if not isinstance(devices, list):
            raise ValueError("invalid simulator group")
        for device in devices:
            identifier = device["udid"]
            if not SIMULATOR_ID.fullmatch(identifier) or identifier.lower() in devices_by_id:
                raise ValueError("invalid simulator inventory ID")
            name = device.get("name")
            if name is not None and not isinstance(name, str):
                raise ValueError("invalid simulator inventory label")
            devices_by_id[identifier.lower()] = name
    return devices_by_id


def recover_owned_allocations(simctl, uncertain):
    pending = uncertain.copy()
    owned = []
    for attempt in range(3):
        if not pending:
            break
        if attempt:
            time.sleep(0.15)
        try:
            inventory = simulator_inventory(simctl)
        except (OSError, subprocess.SubprocessError, ValueError, KeyError, TypeError):
            continue
        for label, before in list(pending.items()):
            matches = [identifier for identifier, name in inventory.items() if name == label]
            if len(matches) == 1 and matches[0] not in before:
                owned.append(matches[0])
                del pending[label]
    return owned, sorted(pending)


def retire_owned_simulators(simctl, owned):
    pending = {identifier.lower() for identifier in owned}
    for _ in range(3):
        if not pending:
            break
        for identifier in sorted(pending):
            for action in ("shutdown", "delete"):
                try:
                    simctl(action, identifier)
                except (OSError, subprocess.SubprocessError):
                    # An unknown tool outcome must be reconciled by inventory.
                    continue
        try:
            pending.intersection_update(simulator_inventory(simctl))
        except (OSError, subprocess.SubprocessError, ValueError, KeyError, TypeError):
            # A successful exit with malformed output cannot prove absence.
            continue
    return sorted(pending)
