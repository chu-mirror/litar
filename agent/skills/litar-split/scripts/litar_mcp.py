"""Stdio client for the litar MCP server (`litar -a`) and helpers for blocks:// dumps.

Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
"""
import json
import os
import re
import subprocess

START = re.compile(r"@[<\[(]")


class Client:
    def __init__(self):
        self.p = subprocess.Popen([os.environ.get("LITAR", "litar"), "-a"],
                                  stdin=subprocess.PIPE, stdout=subprocess.PIPE)
        self.id = 0
        self.call("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                                 "clientInfo": {"name": "litar-split", "version": "0"}})
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
                    raise SystemExit(f"{method}: {json.dumps(msg['error'])}")
                return msg["result"]

    def read(self, uri):
        r = self.call("resources/read", {"uri": uri})
        return "".join(c.get("text", "") for c in r["contents"])

    def tool(self, _tool, **args):
        r = self.call("tools/call", {"name": _tool, "arguments": args})
        text = "".join(c.get("text", "") for c in r.get("content", []))
        if r.get("isError"):
            raise SystemExit(f"{_tool} failed: {text}")
        return text

    def tree(self, uri):
        return json.loads(self.read(uri))


def leaves(tree, prefix=()):
    """Yield (path, id) for the leaves of a tree:// result."""
    path = prefix + (tree["name"],) if tree["name"] not in ("", "/") else prefix
    if not tree["sub-trees"]:
        yield path, tree["id"]
    for sub in tree["sub-trees"]:
        yield from leaves(sub, path)


def all_nodes(tree, prefix=()):
    """Yield (path, id) for every node of a tree://node result."""
    path = prefix + (tree["name"],)
    yield path, tree["id"]
    for sub in tree["sub-trees"]:
        yield from all_nodes(sub, path)


def node_id(cl, c, path):
    ids = dict(all_nodes(cl.tree(f"tree://node/{c}")))
    if tuple(path) not in ids:
        raise SystemExit(f"node {'/'.join(path)} not found")
    return ids[tuple(path)]


def evaluate_files(cl, c, n):
    """Return {path: evaluated text} for every file of node n."""
    return {"/".join(p): cl.read(f"file://{c}/{n}/{fid}")
            for p, fid in leaves(cl.tree(f"tree://file/{c}/{n}"))}


def _escaped(text, k):
    n = 0
    while k - 1 - n >= 0 and text[k - 1 - n] == "@":
        n += 1
    return n % 2 == 1


def split_archive(text):
    """Split archive text into [(comment, block)] and the trailing text.

    A comment is everything between the previous block's end control and the header,
    so joining comment + block in order reproduces the text exactly.
    """
    out, pos = [], 0
    while True:
        m = START.search(text, pos)
        while m and _escaped(text, m.start()):
            m = START.search(text, m.start() + 1)
        if not m:
            return out, text[pos:]
        i = m.start() + 1
        while True:
            j = text.index("@", i)
            nxt = text[j + 1] if j + 1 < len(text) else ""
            if nxt == "" or nxt in " \t\r\n":
                break
            i = j + 2
        out.append((text[pos:m.start()], text[m.start():j + 1]))
        pos = j + 1


def header(block):
    return block.split("\n", 1)[0]


def first_line(block):
    parts = block.split("\n", 2)
    return parts[1] if len(parts) > 1 else ""


def content(block):
    """The block's content: after @= or @+, before the pipeline, items, or end control."""
    m = re.search(r"@[=+]", block)
    i = m.end()
    if block[i:i + 1] == "\n":
        i += 1
    j = i
    while True:
        k = block.index("@", j)
        nxt = block[k + 1:k + 2]
        if nxt == "<":
            j = block.index("@>", k + 2) + 2
        elif nxt == "@":
            j = k + 2
        elif nxt in ("|", "~", "") or nxt in " \t\r\n":
            end = k
            break
        else:
            j = k + 2
    body = block[i:end]
    return body[:-1] if body.endswith("\n") else body
