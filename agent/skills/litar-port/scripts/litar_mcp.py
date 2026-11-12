"""Stdio client for the litar MCP server (`litar -a`).

Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
"""
import json
import os
import subprocess


class ToolError(Exception):
    pass


class Client:
    def __init__(self):
        self.p = subprocess.Popen([os.environ.get("LITAR", "litar"), "-a"],
                                  stdin=subprocess.PIPE, stdout=subprocess.PIPE)
        self.id = 0
        self.call("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                                 "clientInfo": {"name": "litar-script", "version": "0"}})
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def send(self, msg):
        self.p.stdin.write((json.dumps(msg) + "\n").encode())
        self.p.stdin.flush()

    def call(self, method, params):
        self.id += 1
        self.send({"jsonrpc": "2.0", "id": self.id, "method": method, "params": params})
        while True:
            line = self.p.stdout.readline()
            if not line:
                raise SystemExit("litar server closed")
            msg = json.loads(line)
            if msg.get("id") == self.id:
                if "error" in msg:
                    raise ToolError(f"{method}: {json.dumps(msg['error'])}")
                return msg["result"]

    def read(self, uri):
        r = self.call("resources/read", {"uri": uri})
        return "".join(c.get("text", "") for c in r["contents"])

    def json(self, uri):
        return json.loads(self.read(uri))

    def tool(self, _tool, **args):
        r = self.call("tools/call", {"name": _tool, "arguments": args})
        text = "".join(c.get("text", "") for c in r.get("content", []))
        if r.get("isError"):
            raise ToolError(f"{_tool} failed: {text}")
        return text


def entries(tree, prefix=()):
    """Yield (path, ref, is_leaf) for every entry below the root of a tree:// result."""
    for sub in tree["sub-trees"]:
        path = prefix + (sub["name"],)
        yield path, sub["ref"], not sub["sub-trees"]
        yield from entries(sub, path)


def leaves(tree):
    """Yield (path, ref) for the leaves of a tree:// result."""
    return ((path, ref) for path, ref, leaf in entries(tree) if leaf)


def node_ref(cl, c, path):
    """The ref of the node whose names from the root are path."""
    tree = cl.json(f"tree://node/{c}")
    nodes = {(tree["name"],): tree["ref"]}
    nodes.update({(tree["name"],) + p: ref for p, ref, _ in entries(tree)})
    if tuple(path) not in nodes:
        raise SystemExit(f"node {'/'.join(path)} not found")
    return nodes[tuple(path)]


def evaluate_files(cl, c, n):
    """Return {path: evaluated text} for every file of node n."""
    return {"/".join(p): cl.read(f"file://{c}/{n}/{ref}")
            for p, ref in leaves(cl.json(f"tree://file/{c}/{n}"))}
