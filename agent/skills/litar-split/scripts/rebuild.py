#!/usr/bin/env python3
"""Rebuild a leaf node through the litar MCP server, replacing blocks in place.

usage: rebuild.py [-n NODE] [-o OUT] [--files DIR] [--same] [--dry-run] COMPOSITION_DIR PATCH...

A patch file holds groups separated by lines of '%%'. The first block of a group
replaces the node's own block with the same header and first content line; the whole
group takes its place, in order. A comment before the first block replaces the old
comment; without one the old comment stays.

The node (default: the only local node; it must have no descendants) is deleted,
re-created with the same name, archive, description, and base, and every block is
re-applied in its original order, so the archive keeps its layout. Every file of the
node is evaluated before and after; OUT receives changes.diff (a unified diff of the
files that changed) and before.la and after.la, the node's blocks:// dumps.
--files DIR writes every evaluated file under DIR, to build and test them
(file modes are not kept). --same exits 1 if any file changed.
--dry-run checks the patches and stops.
Restore the archive and composer.json from version control if a run fails midway.
Environment: LITAR (default: litar on PATH), LITAR_LIBRARY for grounds.
"""
import argparse
import difflib
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from litar_mcp import Client, evaluate_files, first_line, header, node_id, split_archive  # noqa: E402


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


def load_patches(paths):
    groups = {}
    for path in paths:
        for text in open(path).read().split("\n%%\n"):
            blocks, _ = split_archive(text)
            if not blocks:
                continue
            key = (header(blocks[0][1]), first_line(blocks[0][1]))
            if key in groups:
                raise SystemExit(f"two patches for {key}")
            groups[key] = blocks
    return groups


def plan(own, groups):
    """Return [(comment, block)] for the rebuilt node."""
    blocks, tail = split_archive(own)
    if tail.strip():
        raise SystemExit("unexpected text after the last block")
    present = {(header(b), first_line(b)) for _, b in blocks}
    for g in groups.values():
        for _, gb in g[1:]:
            if (header(gb), first_line(gb)) in present:
                raise SystemExit(f"already in the node, the patch was applied before: {header(gb)}")
    seq, used = [], set()
    for cm, b in blocks:
        key = (header(b), first_line(b))
        if key not in groups:
            seq.append((cm, b))
            continue
        used.add(key)
        g = groups[key]
        new_cm = g[0][0].strip("\n")
        seq.append(("\n\n" + new_cm + "\n" if new_cm else cm, g[0][1]))
        for gcm, gb in g[1:]:
            seq.append(("\n\n" + gcm.strip("\n") + "\n" if gcm.strip() else "\n\n", gb))
    if set(groups) - used:
        raise SystemExit(f"patches matching no block: {sorted(set(groups) - used)}")
    if seq and seq[0][0].startswith("\n\n") and not blocks[0][0].startswith("\n"):
        seq[0] = (seq[0][0].lstrip("\n"), seq[0][1])
    return seq


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("-n", "--node")
    ap.add_argument("-o", "--out", default="litar-split-out")
    ap.add_argument("--files")
    ap.add_argument("--same", action="store_true")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("composition")
    ap.add_argument("patches", nargs="+")
    a = ap.parse_args()
    comp = os.path.abspath(a.composition)
    node = local_node(comp, a.node)
    path = node["base"] + [node["name"]]
    composer = json.load(open(os.path.join(comp, "composer.json")))
    if any(m["base"][:len(path)] == path for m in composer["nodes"]):
        raise SystemExit(f"{node['name']} has descendants; deleting it would delete them")
    groups = load_patches(a.patches)

    cl = Client()
    c = int(cl.tool("import_composition", path=comp))
    n = node_id(cl, c, path)
    base = node_id(cl, c, node["base"]) if node["base"] else None
    own = cl.read(f"blocks://node/{c}/{n}")
    seq = plan(own, groups)
    print(f"blocks {len(split_archive(own)[0])} -> {len(seq)}")
    if a.dry_run:
        return

    os.makedirs(a.out, exist_ok=True)
    open(os.path.join(a.out, "before.la"), "w").write(own)
    before = evaluate_files(cl, c, n)
    composer_bytes = open(os.path.join(comp, "composer.json"), "rb").read()
    cl.tool("delete_node", composition=c, node=n)
    cl.tool("create_node", composition=c, name=node["name"], archive=node["archive"],
            description=node["description"], base=base)
    n = node_id(cl, c, path)
    for cm, b in seq:
        cl.tool("apply_block", composition=c, node=n, text=cm + b)

    after = evaluate_files(cl, c, n)
    open(os.path.join(a.out, "after.la"), "w").write(cl.read(f"blocks://node/{c}/{n}"))
    changed = sorted(p for p in set(before) | set(after) if before.get(p) != after.get(p))
    with open(os.path.join(a.out, "changes.diff"), "w") as f:
        for p in changed:
            f.writelines(difflib.unified_diff(
                before.get(p, "").splitlines(True), after.get(p, "").splitlines(True),
                "a/" + p, "b/" + p))
    if a.files:
        for p, text in after.items():
            dest = os.path.join(a.files, p)
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            open(dest, "w").write(text)
    if open(os.path.join(comp, "composer.json"), "rb").read() != composer_bytes:
        print("composer.json changed (node order or formatting); check it")
    print(f"files {len(before)}, changed: {', '.join(changed) or 'none'}")
    sys.exit(1 if a.same and changed else 0)


if __name__ == "__main__":
    main()
