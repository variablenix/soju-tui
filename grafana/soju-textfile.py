#!/usr/bin/env python3
"""Optional, read-only Soju 0.10.1 aggregate collector for node_exporter."""

import argparse
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import sys
import tempfile
import time

MAX_OUTPUT = 1024 * 1024
SERVER = re.compile(r"(\d+)/(\d+) users, (\d+) downstreams, (\d+) upstreams, (\d+) networks, (\d+) channels")
USER = re.compile(r"(.+): (\d+) networks(?: \(\d+ max\))?")
ATTRIBUTES = (" (admin, disabled)", " (disabled, admin)", " (admin)", " (disabled)")
NETWORK = re.compile(r".+? \[(connected|disabled|disconnected)\](?:: (.*))?")


class CollectionError(Exception):
    """Deliberately contains no command output or account information."""


def command(argv, deadline):
    """Bound both wall time and combined output; never invoke a shell."""
    with subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT) as proc:
        try:
            data = bytearray()
            with selectors.DefaultSelector() as selector:
                selector.register(proc.stdout, selectors.EVENT_READ)
                while True:
                    remaining = deadline - time.monotonic()
                    if remaining <= 0 or not selector.select(remaining):
                        raise CollectionError("command timed out")
                    chunk = os.read(proc.stdout.fileno(), 65536)
                    if not chunk:
                        break
                    data.extend(chunk)
                    if len(data) > MAX_OUTPUT:
                        raise CollectionError("command output exceeded limit")
            try:
                code = proc.wait(timeout=max(0.001, deadline - time.monotonic()))
            except subprocess.TimeoutExpired:
                raise CollectionError("command timed out") from None
            if code:
                raise CollectionError("read-only command failed")
            return data.decode("utf-8", errors="strict")
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()


def server_status(text):
    match = SERVER.fullmatch(text.strip())
    if not match:
        raise CollectionError("unrecognized server status")
    active, users, clients, upstreams, networks, channels = map(int, match.groups())
    return users, networks, channels


def users_status(text, expected):
    users = []
    for line in text.splitlines():
        match = USER.fullmatch(line)
        if not match:
            raise CollectionError("incomplete or unrecognized user listing")
        name, count = match.groups()
        for suffix in ATTRIBUTES:
            if name.endswith(suffix):
                name = name[:-len(suffix)]
                break
        if not name or name in [u[0] for u in users] or any(ord(c) < 32 for c in name):
            raise CollectionError("ambiguous user listing")
        users.append((name, int(count)))
    if len(users) != expected:
        raise CollectionError("incomplete user listing")
    return users


def network_status(text, expected):
    if expected == 0 and text.strip() == 'No network configured, add one with "network create".':
        return 0, 0, 0
    lines = text.splitlines()
    if len(lines) != expected:
        raise CollectionError("incomplete network listing")
    connected = joined = disabled = 0
    for line in lines:
        match = NETWORK.fullmatch(line)
        if not match:
            raise CollectionError("unrecognized network status")
        state, detail = match.groups()
        if state == "connected":
            channels = re.fullmatch(r"(\d+) channels", detail or "")
            if not channels:
                raise CollectionError("unrecognized connected-channel count")
            connected += 1
            joined += int(channels[1])
        elif state == "disabled":
            if detail is not None:
                raise CollectionError("unrecognized disabled-network status")
            disabled += 1
    return connected, joined, disabled


def collect(run):
    before = server_status(run(["server", "status"]))
    users = users_status(run(["user", "status"]), before[0])
    if sum(n for _, n in users) != before[1]:
        raise CollectionError("configuration changed during collection")
    connected = joined = disabled = 0
    for user, count in users:
        c, j, d = network_status(run(["user", "run", user, "network", "status"]), count)
        connected += c
        joined += j
        disabled += d
    if before != server_status(run(["server", "status"])):
        raise CollectionError("configuration changed during collection")
    return {"networks_configured": before[1], "networks_connected": connected,
            "networks_disabled": disabled, "networks_disconnected": before[1] - connected - disabled,
            "channels_configured": before[2], "channels_joined": joined}


def render(instance, counts, timestamp):
    # JSON string escaping is compatible with Prometheus label escaping for
    # these labels. Reject controls rather than emit JSON-only escape forms.
    if not instance or any(ord(c) < 32 or ord(c) == 127 for c in instance):
        raise CollectionError("invalid instance label")
    label = '{soju_instance=' + json.dumps(instance, ensure_ascii=False) + '}'
    values = {"collection_success": int(counts is not None), "collected_timestamp_seconds": timestamp}
    if counts is not None:
        values.update(counts)
    lines = []
    for name, value in values.items():
        metric = "soju_admin_" + name
        lines.extend([f"# HELP {metric} Optional read-only sojuctl aggregate ({name}).",
                      f"# TYPE {metric} gauge", f"{metric}{label} {value}"])
    return "\n".join(lines) + "\n"


def write_atomic(output, content):
    # The containing directory must be dedicated and administrator-owned.
    path = Path(output)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", prefix=".soju-",
                                         dir=path.parent, delete=False) as stream:
            temporary = stream.name
            stream.write(content)
            stream.flush()
            os.fchmod(stream.fileno(), 0o644)
        os.replace(temporary, path)
    finally:
        if temporary and os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--sojuctl", default="/usr/bin/sojuctl")
    parser.add_argument("--config", default="/etc/soju/config")
    parser.add_argument("--instance", required=True, help="exact instance label of the native soju scrape")
    parser.add_argument("--output", required=True, help="file ending in .prom in a dedicated trusted directory")
    args = parser.parse_args()
    if not os.path.isabs(args.sojuctl) or not args.output.endswith(".prom"):
        parser.error("sojuctl must be absolute and output must end in .prom")
    deadline = time.monotonic() + 20
    counts = None
    try:
        counts = collect(lambda words: command([args.sojuctl, "-config", args.config] + words, deadline))
    except (CollectionError, OSError, UnicodeError) as error:
        # Never log output, names, addresses, or upstream error details.
        print("Soju aggregate collection failed (" + type(error).__name__ + ")", file=sys.stderr)
    write_atomic(args.output, render(args.instance, counts, int(time.time())))
    return int(counts is None)


if __name__ == "__main__":
    sys.exit(main())
