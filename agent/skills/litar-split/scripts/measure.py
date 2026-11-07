#!/usr/bin/env python3
"""List the own blocks of a node longer than a token limit, longest first.

usage: measure.py [-n NODE] [-l LIMIT] COMPOSITION_DIR

NODE is the node name (default: the only local node); LIMIT defaults to 150.
Tokens are estimated as content characters / 4. Each line shows the estimate,
the line count, the block's line in the node's blocks:// dump, and its header.
Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from litar_mcp import Client, content, header, node_id, split_archive  # noqa: E402
from rebuild import local_node  # noqa: E402


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", "--node")
    ap.add_argument("-l", "--limit", type=int, default=150)
    ap.add_argument("composition")
    a = ap.parse_args()
    comp = os.path.abspath(a.composition)
    node = local_node(comp, a.node)
    cl = Client()
    c = int(cl.tool("import_composition", path=comp))
    n = node_id(cl, c, node["base"] + [node["name"]])
    text = cl.read(f"blocks://node/{c}/{n}")
    blocks, _ = split_archive(text)
    rows, line = [], 1
    for cm, b in blocks:
        line += cm.count("\n")
        body = content(b)
        rows.append((len(body) // 4, body.count("\n") + 1, line, header(b)))
        line += b.count("\n")
    long = sorted((r for r in rows if r[0] > a.limit), reverse=True)
    print(f"blocks {len(rows)}, over {a.limit} tokens: {len(long)}")
    for tok, lines, ln, h in long:
        print(f"{tok:5d} {lines:4d}L line {ln:6d} {h}")


if __name__ == "__main__":
    main()
