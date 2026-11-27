"""Private scratch space and PostgreSQL shutdown for local integration runners."""

from contextlib import ExitStack, contextmanager
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time


_TERMINATION_SIGNALS = (signal.SIGTERM, signal.SIGHUP)
_INTERRUPTION_SIGNALS = (*_TERMINATION_SIGNALS, signal.SIGINT)


class WorkspaceCleanupError(RuntimeError):
    """Owned resources may still be alive. Keep the workspace for diagnosis."""


@contextmanager
def _defer_interruptions():
    if threading.current_thread() is not threading.main_thread():
        yield
        return
    pending = []

    def defer(signum, frame):
        if not pending:
            pending.append((signum, frame))

    previous = {}
    for sig in _INTERRUPTION_SIGNALS:
        # An ignored signal cannot consume the one deferred interruption.
        if signal.getsignal(sig) != signal.SIG_IGN:
            previous[sig] = signal.signal(sig, defer)
    try:
        yield
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)
        if pending and not sys.exc_info()[0]:
            signum, frame = pending[0]
            handler = previous[signum]
            if callable(handler):
                handler(signum, frame)
            elif handler == signal.SIG_DFL:
                raise SystemExit(128 + signum)


@contextmanager
def _ignore_interruptions():
    if threading.current_thread() is not threading.main_thread():
        yield
        return
    previous = {sig: signal.signal(sig, signal.SIG_IGN) for sig in _INTERRUPTION_SIGNALS}
    try:
        yield
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)


@contextmanager
def cleanup_stack():
    stack = ExitStack()
    try:
        yield stack
    finally:
        with _ignore_interruptions():
            stack.close()


def _await_group_disappearance(group, timeout=3):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            os.killpg(group, 0)
        except ProcessLookupError:
            return True
        except PermissionError:
            pass
        except OSError:
            return False
        time.sleep(0.05)
    return False


def _signal_group(child, signum):
    try:
        os.killpg(child.pid, signum)
    except ProcessLookupError:
        pass
    except OSError as failure:
        # Darwin can return EPERM for a group containing only exiting zombies.
        # Confirm the group disappears; persistent permission failures retain scratch.
        if sys.platform == "darwin" and isinstance(failure, PermissionError):
            child.poll()
            if _await_group_disappearance(child.pid):
                return
        raise WorkspaceCleanupError(f"Could not signal owned process group {child.pid}: {failure}") from failure


def stop_owned_process(child, grace=5):
    """Stop and reap a child started in a fresh session, including descendants."""
    _signal_group(child, signal.SIGTERM)
    try:
        child.wait(timeout=grace)
    except subprocess.TimeoutExpired:
        pass
    # An exited leader may leave children in the same group.
    _signal_group(child, signal.SIGKILL)
    try:
        child.wait(timeout=grace)
    except subprocess.TimeoutExpired as failure:
        raise WorkspaceCleanupError(f"Could not reap owned process group {child.pid}") from failure


class OwnedProcess:
    """One restartable owned child retained until its stop has been confirmed."""

    def __init__(self):
        self.child = None

    def start(self, args, **options):
        with _defer_interruptions():
            self.stop()
            self.child = subprocess.Popen(args, start_new_session=True, **options)
        return self.child

    def stop(self):
        if self.child is not None:
            stop_owned_process(self.child)
            self.child = None


def run_command(args, *, check=False, capture_output=False, timeout=None, input=None, termination_grace=None, **options):
    if capture_output:
        options.update(stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if input is not None:
        options.setdefault("stdin", subprocess.PIPE)
    child = None
    try:
        with _defer_interruptions():
            child = subprocess.Popen(args, start_new_session=True, **options)
        out, err = child.communicate(input=input, timeout=timeout)
    except BaseException:
        if child is not None:
            with _ignore_interruptions():
                if termination_grace is not None:
                    stop_owned_process(child, grace=termination_grace)
                else:
                    _signal_group(child, signal.SIGKILL)
                    try:
                        child.wait(timeout=5)
                    except subprocess.TimeoutExpired as failure:
                        raise WorkspaceCleanupError(f"Could not reap owned command group {child.pid}") from failure
        raise
    finally:
        if child is not None:
            for stream in (child.stdin, child.stdout, child.stderr):
                if stream is not None:
                    stream.close()
    result = subprocess.CompletedProcess(args, child.returncode, out, err)
    if check:
        result.check_returncode()
    return result


def check_output(args, **options):
    return run_command(args, stdout=subprocess.PIPE, check=True, **options).stdout


def start_postgres(pg_ctl, data, log, options, env, *, cwd=None, stdout=None, stderr=None):
    # pg_ctl launches a detached postmaster. Complete its bounded startup
    # handshake before processing a signal so cleanup cannot race launch.
    wait = 60
    started = time.monotonic()
    try:
        with _defer_interruptions():
            run_command([pg_ctl, "-D", data, "-l", log, "-o", options, "-w", "-t", str(wait), "start"],
                        env=env, cwd=cwd, stdout=stdout, stderr=stderr, timeout=wait + 5, check=True)
    except subprocess.TimeoutExpired as failure:
        raise WorkspaceCleanupError("PostgreSQL launcher did not complete its startup handshake") from failure
    except subprocess.CalledProcessError as failure:
        if time.monotonic() - started >= wait:
            raise WorkspaceCleanupError("PostgreSQL launcher timed out before proving startup") from failure
        raise


def run_environment(work, environment):
    env = environment.copy()
    temporary = work / "tmp"
    temporary.mkdir(mode=0o700)
    cache = work / "go-cache"
    cache.mkdir(mode=0o700)
    env.update(TMPDIR=str(temporary), TEMP=str(temporary), TMP=str(temporary),
               GOTMPDIR=str(temporary), GOCACHE=str(cache), PYTHONDONTWRITEBYTECODE="1")
    env.pop("GOCACHEPROG", None)
    def go_environment(*keys, settings=env):
        output = run_command(["go", "env", "-json", *keys], env=settings,
                             capture_output=True, text=True, check=True).stdout
        return json.loads(output)

    preferences = go_environment("GOENV", "GOCACHEPROG")
    if not preferences["GOCACHEPROG"]:
        return env

    # Preserve recognized Go-only preferences when the user config selects an
    # external cache program. A regular run reads its existing config directly.
    location = preferences["GOENV"]
    if not location:
        raise RuntimeError("Cannot disable the configured external Go cache program")
    defaults = {**env, "GOENV": "off"}
    if go_environment("GOCACHEPROG", settings=defaults)["GOCACHEPROG"]:
        raise RuntimeError("Cannot disable the configured external Go cache program")
    understood = set(go_environment(settings=defaults))
    # Go reports architecture-specific keys only for the selected architecture.
    understood.update("FC GO386 GOAMD64 GOARM GOMIPS GOMIPS64 GOPPC64 GORISCV64 GOWASM GO_EXTLINK_ENABLED".split())
    for mode in ("CFLAGS", "CPPFLAGS", "CXXFLAGS", "FFLAGS", "LDFLAGS"):
        understood.update((f"CGO_{mode}_ALLOW", f"CGO_{mode}_DISALLOW"))
    excluded = {"GOENV", "GOCACHE", "GOTMPDIR", "GOCACHEPROG"}
    allowed = {name.encode() for name in understood - excluded}
    config = work / "go.env"
    with os.fdopen(os.open(config, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as output:
        for line in Path(location).read_bytes().split(b"\n"):
            key, separator, _ = line.partition(b"=")
            if separator and key in allowed:
                output.write(line + b"\n")
    env["GOENV"] = str(config)
    if go_environment("GOCACHEPROG")["GOCACHEPROG"]:
        raise RuntimeError("Cannot disable the configured external Go cache program")
    return env


def stop_postgres(pg_ctl, data, env):
    def control(*args):
        try:
            return subprocess.run([pg_ctl, "-D", data, *args], env=env,
                                  stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                  timeout=15).returncode
        except (OSError, subprocess.TimeoutExpired):
            return None

    if not (data / "PG_VERSION").exists() and not (data / "postmaster.pid").exists():
        return
    initial = control("status")
    if initial == 3:
        return
    checks = [f"initial status={initial}"]
    for mode in ("fast", "immediate"):
        result = control("-m", mode, "-w", "-t", "10", "stop")
        status = control("status")
        if status == 3:
            return
        checks.append(f"{mode} stop={result}, status={status}")
    raise WorkspaceCleanupError(
        f"Could not confirm PostgreSQL shutdown ({'; '.join(checks)}). "
        f"Retained integration workspace at {data.parent}"
    )


@contextmanager
def scoped_workspace(prefix, *, directory=None, term_status=143):
    """Remove only the owned scratch after its consumers have stopped."""
    work = None

    def exit_on_signal(signum, _frame):
        for sig in _TERMINATION_SIGNALS:
            signal.signal(sig, signal.SIG_IGN)
        raise SystemExit(term_status if signum == signal.SIGTERM else 128 + signum)

    # An invocation deliberately launched under nohup keeps its inherited policy.
    handled = (signal.SIGTERM,) if signal.getsignal(signal.SIGHUP) == signal.SIG_IGN else _TERMINATION_SIGNALS
    previous = {sig: signal.signal(sig, exit_on_signal) for sig in handled}
    retain = False
    try:
        with _defer_interruptions():
            work = Path(tempfile.mkdtemp(prefix=prefix, dir=directory))
        yield work
    except WorkspaceCleanupError:
        retain = True
        raise
    finally:
        try:
            with _ignore_interruptions():
                if work is not None:
                    if retain:
                        print(f"Retained integration workspace at {work}", file=sys.stderr)
                    else:
                        shutil.rmtree(work)
        finally:
            for sig, handler in previous.items():
                signal.signal(sig, handler)


@contextmanager
def integration_workspace(prefix, pg_ctl, data_name, environment, *, term_status=143):
    # AF_UNIX socket addresses have a short fixed-size limit.
    with scoped_workspace(prefix, directory="/tmp", term_status=term_status) as work:
        env = environment
        try:
            env = run_environment(work, environment)
            yield work, env
        finally:
            with _ignore_interruptions():
                stop_postgres(pg_ctl, work / data_name, env)
