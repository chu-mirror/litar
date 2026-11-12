#!/usr/bin/env python3
"""List the own blocks of a node longer than a token limit, longest first.

usage: measure.py [-n NODE] [-l LIMIT] COMPOSITION_DIR

NODE is the node name (default: the only local node); LIMIT defaults to 150.
Tokens are estimated as content characters / 4; comments and items are not counted.
Each line shows the estimate, the line count, the block's ref, and its header.
Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
"""
import argparse
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from litar_mcp import Client, node_ref  # noqa: E402


def local_node(comp, name):
    nodes = json.load(open(os.path.join(comp, "composer.json")))["nodes"]
    if name is None:
        if len(nodes) != 1:
            raise SystemExit("several local nodes; pass -n NODE")
        return nodes[0]
    for node in nodes:
        if node["name"] == name:
            return node
    raise SystemExit(f"no local node named {name}")


def content(block):
    """The block's content: after @= or @+, before the pipeline, items, or end control."""
    i = re.search(r"@[=+]", block).end()
    if block[i:i + 1] == "\n":
        i += 1
    j = i
    while True:
        k = block.index("@", j)
        nxt = block[k + 1:k + 2]
        if nxt == "<":
            j = block.index("@>", k + 2) + 2
        elif nxt in ("|", "~", "") or nxt in " \t\r\n":
            break
        else:
            j = k + 2
    body = block[i:k]
    return body[:-1] if body.endswith("\n") else body


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", "--node")
    ap.add_argument("-l", "--limit", type=int, default=150)
    ap.add_argument("composition")
    a = ap.parse_args()
    comp = os.path.abspath(a.composition)
    node = local_node(comp, a.node)
    cl = Client()
    c = cl.tool("import_composition", path=comp)
    n = node_ref(cl, c, node["base"] + [node["name"]])
    rows = []
    for b in cl.json(f"blocks://node/{c}/{n}"):
        body = content(b["text"])
        rows.append((len(body) // 4, body.count("\n") + 1, b["ref"], b["text"].split("\n", 1)[0]))
    long = sorted((r for r in rows if r[0] > a.limit), reverse=True)
    print(f"blocks {len(rows)}, over {a.limit} tokens: {len(long)}")
    for tok, lines, ref, head in long:
        print(f"{tok:5d} {lines:4d}L {ref} {head}")


if __name__ == "__main__":
    main()
