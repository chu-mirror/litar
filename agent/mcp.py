#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "mcp>=2.3",
# ]
# ///
"""MCP server that inspects and edits a litar archive.

Run it with ``uv run agent/mcp.py`` from the repository, or point a client at
``uv run --script`` plus the absolute path of this file. The server speaks
MCP over stdio.

The server does not parse an archive itself. ``litar`` answers every read.
``sed`` performs every edit. ``litar -a`` supplies the file and the line
range of each block. Set ``LITAR`` to choose the executable; otherwise the
server uses the ``litar`` built beside ``src/``, then ``litar`` on ``PATH``.
"""

import sys
from pathlib import Path

# Running this file puts its directory, src/, first on sys.path. That would
# shadow the installed mcp package, because this file is named mcp.py.
_script_dir = Path(__file__).resolve().parent
sys.path[:] = [
    entry for entry in sys.path if not entry or Path(entry).resolve() != _script_dir
]

import json
import os
import shutil
import subprocess
import tempfile
import threading
from typing import Annotated, Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator

from mcp.server import MCPServer
from mcp.server.mcpserver.exceptions import ToolError
from mcp.types import ToolAnnotations

_LOCK = threading.Lock()
_READ_ONLY = ToolAnnotations(read_only_hint=True, open_world_hint=False)
_OVERWRITE = ToolAnnotations(
    read_only_hint=False,
    destructive_hint=True,
    idempotent_hint=False,
    open_world_hint=False,
)
_APPEND = ToolAnnotations(
    read_only_hint=False,
    destructive_hint=False,
    idempotent_hint=False,
    open_world_hint=False,
)

INSTRUCTIONS = """
This server inspects and edits one litar archive at a time.
Do not open a .la file yourself; send the operations on a .la file to this server.

A chunk reference looks like label@:module name@/chunk and has no closing @.
An expression may continue with filters: label@:module name@/chunk@|filter.
In those strings, write a literal @ inside a name as @@.

list_modules returns every module and the evaluated text of its summary chunk.
blocks_applied is litar -a -l. blocks_effective is litar -a -e. blocks_depending is litar -a -d.
evaluate_expression is litar -p. Print and modules stay plain text; -a does not change them.
modify_block and delete_block address a block with the file and the line numbers from a listing.
Fetch the listing again after an edit, because line numbers move.
append_block writes the block at the end of the archive.
It first selects that block's module with @-, so a reference in the body that omits a module stays in that module.
content is the raw body stored in the archive, not the evaluated text. @@ in content is a literal @.
The newline after @= or @+ is layout and is not part of content.
""".strip()


class Filter(BaseModel):
    """One filter in a pipeline. Names are decoded: one @ is a literal @."""

    labels: list[str] = Field(default_factory=list)
    module: str = ""
    chunk: str


class BlockRecord(BaseModel):
    """One block, in the shape litar -a prints."""

    model_config = ConfigDict(populate_by_name=True, serialize_by_alias=True)

    comment: str
    labels: list[str]
    module: str
    chunk: str
    type: Literal["set", "extend"]
    content: str
    filters: list[Filter]
    file: str
    line_range: list[int] = Field(alias="line-range", min_length=2, max_length=2)

    @field_validator("line_range")
    @classmethod
    def _span(cls, value: list[int]) -> list[int]:
        if value[0] < 1 or value[1] < value[0]:
            raise ValueError("line-range must be a positive inclusive span")
        return value


class ModuleInfo(BaseModel):
    """A module and the evaluated text of its summary chunk."""

    module: str = Field(description="Module name. Empty for the anonymous module.")
    description: str | None = Field(
        description="Evaluated summary. Null when the module defines no summary."
    )
    error: str | None = Field(
        default=None,
        description="litar's message when summary is defined but could not be evaluated.",
    )


class Evaluation(BaseModel):
    """The text litar -p wrote, plus any stderr from a successful run."""

    content: str
    stderr: str = ""


class EditResult(BaseModel):
    """Where an edit landed. Fetch the block again for its line-range."""

    file: str
    start_line: int
    end_line: int
    blocks: list[BlockRecord] = Field(
        default_factory=list,
        description="Blocks now applied to the written chunk, from litar -a -l on this file. Empty after a delete.",
    )


def _tool_error_from(proc: subprocess.CompletedProcess[bytes], program: str) -> ToolError:
    err = proc.stderr.decode("utf-8", "replace").strip()
    out = proc.stdout.decode("utf-8", "replace").strip()
    detail = err or out or f"exited with status {proc.returncode}"
    return ToolError(f"{program}: {detail}")


def litar_program() -> str:
    """Resolve the litar executable: $LITAR, then the binary beside src/, then PATH."""

    chosen = os.environ.get("LITAR")
    if chosen:
        if os.path.isfile(chosen) and os.access(chosen, os.X_OK):
            return chosen
        raise ToolError(f"LITAR={chosen} is not an executable litar")
    sibling = _script_dir.parent / "litar"
    if sibling.is_file() and os.access(sibling, os.X_OK):
        return str(sibling)
    found = shutil.which("litar")
    if found:
        return found
    raise ToolError("could not find litar. Build it with make, put it on PATH, or set LITAR")


def run_litar(*args: str) -> subprocess.CompletedProcess[bytes]:
    """Run litar. Stdin is closed so a filter cannot read the MCP stream."""

    return subprocess.run(
        [litar_program(), *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        stdin=subprocess.DEVNULL,
        check=False,
    )


def require_litar(*args: str) -> subprocess.CompletedProcess[bytes]:
    proc = run_litar(*args)
    if proc.returncode != 0:
        raise _tool_error_from(proc, "litar")
    return proc


def _sed_env() -> dict[str, str]:
    env = os.environ.copy()
    env["LC_ALL"] = "C"
    env["LANG"] = "C"
    return env


def run_sed(args: list[str], path: str | None = None) -> subprocess.CompletedProcess[bytes]:
    command = ["sed", *args]
    if path is not None:
        command.extend(["--", path])
    proc = subprocess.run(
        command,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        stdin=subprocess.DEVNULL,
        check=False,
        env=_sed_env(),
    )
    if proc.returncode != 0:
        raise _tool_error_from(proc, "sed")
    return proc


def line_count(path: str) -> int:
    """Count LF-terminated lines the way litar does. An empty file has no lines."""

    proc = run_sed(["-n", "$="], path)
    text = proc.stdout.decode("ascii", "replace").strip()
    if not text:
        return 0
    return int(text)


def sed_range(path: str, start: int, end: int) -> bytes:
    return run_sed(["-n", f"{start},{end}p"], path).stdout


def _sed_inplace(script: str, path: str) -> None:
    """Apply a sed script to path. The script is a temp file, not the archive."""

    descriptor, script_path = tempfile.mkstemp(prefix="litar-sed-", suffix=".sed")
    try:
        os.write(descriptor, script.encode("ascii"))
        os.close(descriptor)
        descriptor = -1
        run_sed(["-i", "--follow-symlinks", "-f", script_path], path)
    finally:
        if descriptor >= 0:
            os.close(descriptor)
        os.unlink(script_path)


def _write_blob(text: bytes) -> str:
    descriptor, blob = tempfile.mkstemp(prefix="litar-block-")
    os.write(descriptor, text)
    os.close(descriptor)
    return blob


def snapshot(path: str) -> str:
    """Copy path to a temp file through sed, so a failed edit can be restored."""

    descriptor, blob = tempfile.mkstemp(prefix="litar-snap-")
    os.close(descriptor)
    proc = subprocess.run(
        ["sed", "-n", "p", "--", path],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        stdin=subprocess.DEVNULL,
        check=False,
        env=_sed_env(),
    )
    if proc.returncode != 0:
        os.unlink(blob)
        raise _tool_error_from(proc, "sed")
    with open(blob, "wb") as sink:
        sink.write(proc.stdout)
    return blob


def restore(path: str, blob: str) -> None:
    """Replace path with a sed snapshot. An empty snapshot clears the file."""

    last = line_count(path)
    if last == 0:
        _fill_empty(path, blob)
        return
    _sed_inplace("1{\n1r " + blob + "\nd\n}\n2,$d\n", path)


def _fill_empty(path: str, blob: str) -> None:
    """GNU sed has no address in an empty file, so sed copies the blob onto it.

    This is the only place the archive is opened for writing. sed still
    produces the bytes. A non-empty archive is edited with sed -i instead.
    """

    proc = subprocess.run(
        ["sed", "-n", "p", "--", blob],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        stdin=subprocess.DEVNULL,
        check=False,
        env=_sed_env(),
    )
    if proc.returncode != 0:
        raise _tool_error_from(proc, "sed")
    with open(path, "wb") as sink:
        sink.write(proc.stdout)


def replace_lines(path: str, start: int, end: int, text: str) -> None:
    """Replace the inclusive line span with text. text must be non-empty."""

    payload = text.encode("utf-8")
    if not payload.endswith(b"\n"):
        payload += b"\n"
    blob = _write_blob(payload)
    try:
        _sed_inplace(f"{start},{end}{{\n{start}r {blob}\nd\n}}\n", path)
    finally:
        os.unlink(blob)


def delete_lines(path: str, start: int, end: int) -> None:
    _sed_inplace(f"{start},{end}d\n", path)


def append_lines(path: str, text: str) -> None:
    payload = text.encode("utf-8")
    if not payload.endswith(b"\n"):
        payload += b"\n"
    if line_count(path) == 0:
        blob = _write_blob(payload)
        try:
            _fill_empty(path, blob)
        finally:
            os.unlink(blob)
        return
    blob = _write_blob(payload)
    try:
        _sed_inplace("$r " + blob + "\n", path)
    finally:
        os.unlink(blob)


def _require_archive(path: str) -> None:
    if not path:
        raise ToolError("archive path is empty")
    if not os.path.isfile(path):
        raise ToolError(f"could not open '{path}'")


def _require_span(path: str, start: int, end: int) -> None:
    if start < 1 or end < start:
        raise ToolError(f"line range {start}-{end} is not a positive inclusive span")
    last = line_count(path)
    if end > last:
        raise ToolError(f"'{path}' has {last} lines; the range ends at {end}")


def _edit(path: str, mutate) -> None:
    """Parse with litar, edit with sed, and restore the sed snapshot if parsing fails."""

    _require_archive(path)
    require_litar("-m", path)
    snap = snapshot(path)
    try:
        mutate()
        try:
            require_litar("-m", path)
        except ToolError:
            try:
                restore(path, snap)
            except ToolError as restore_error:
                raise ToolError(
                    f"{restore_error}. The previous text could not be restored."
                ) from restore_error
            raise
    finally:
        os.unlink(snap)


def _text_lines(text: str) -> int:
    if text == "":
        return 0
    count = text.count("\n")
    if not text.endswith("\n"):
        count += 1
    return count


def encode_name(name: str) -> str:
    """Escape @ the way litar writes a name back into an archive."""

    return name.replace("@", "@@")


def encode_reference(labels: list[str], module: str, chunk: str) -> str:
    """Build a chunk reference without the closing @, as the litar CLI spells it."""

    head = [encode_name(label) for label in labels]
    if module:
        tail = encode_name(module) + "@/" + encode_name(chunk)
    else:
        tail = encode_name(chunk)
    if head:
        return "@:".join(head) + "@:" + tail
    return tail


def _clean_name(kind: str, name: str, allow_empty: bool) -> str:
    trimmed = name.strip(" \t\r\n")
    if "\n" in trimmed or "\r" in trimmed:
        raise ToolError(f"{kind} contains a newline")
    if not trimmed and not allow_empty:
        raise ToolError(f"{kind} is empty")
    return trimmed


def _clean_filters(filters: list[Filter] | None) -> list[Filter]:
    cleaned: list[Filter] = []
    for item in filters or []:
        labels = [_clean_name("filter label", label, False) for label in item.labels]
        cleaned.append(
            Filter(
                labels=labels,
                module=_clean_name("filter module", item.module, True),
                chunk=_clean_name("filter chunk", item.chunk, False),
            )
        )
    return cleaned


def format_block(
    comment: str,
    labels: list[str],
    module: str,
    chunk: str,
    block_type: str,
    content: str,
    filters: list[Filter] | None,
) -> str:
    """Serialize one block as archive text.

    content is written unchanged. The caller already holds the raw body,
    including any @@ escapes. A newline is placed after the operator, and
    the block ends at @.
    """

    labels = [_clean_name("label", label, False) for label in labels]
    module = _clean_name("module", module, True)
    chunk = _clean_name("chunk", chunk, False)
    filters = _clean_filters(filters)
    if block_type == "set":
        operator = "@="
    elif block_type == "extend":
        operator = "@+"
    else:
        raise ToolError("type must be set or extend")
    parts: list[str] = []
    if comment:
        if not comment.endswith("\n"):
            comment += "\n"
        parts.append(comment)
    parts.append("@<" + encode_reference(labels, module, chunk) + operator + "\n")
    parts.append(content)
    for item in filters:
        parts.append("@|" + encode_reference(item.labels, item.module, item.chunk))
    parts.append("@\n")
    return "".join(parts)


def _anchor_blank(path: str, start: int, comment: str) -> str:
    """Keep the blank line -a includes when the reported comment is empty.

    That line is the blank line before the header. It is part of the span
    and not part of the comment string. Replacing the span without it would
    delete the spacer above the block.
    """

    if comment != "":
        return ""
    raw = sed_range(path, start, start)
    if not raw.endswith(b"\n"):
        return ""
    line = raw[:-1]
    if line.endswith(b"\r"):
        line = line[:-1]
    if line.strip(b" \t") != b"":
        return ""
    return raw.decode("ascii")


def _module_selection(module: str) -> str:
    """A @- expression that selects module for the block written after it.

    @- @ selects the anonymous module. A header that names a module still
    extends that module, but a reference in the body that omits @/ uses the
    module selected here.
    """

    if module == "":
        return "@- @\n"
    return "@- " + encode_name(module) + " @\n"


def _missing_summary(stderr: str, module: str) -> bool:
    key = "summary" if module == "" else f"{module}@/summary"
    return stderr.strip() == f"Error: chunk '{key}' is not defined"


def _summary_expression(module: str) -> str:
    if module == "":
        return "summary"
    return encode_name(module) + "@/summary"


def _load_blocks(flag: str, reference: str, archive: str) -> list[BlockRecord]:
    if not reference:
        raise ToolError("chunk reference is empty")
    proc = require_litar("-a", flag, reference, archive)
    try:
        data = json.loads(proc.stdout)
    except json.JSONDecodeError as exc:
        raise ToolError(f"litar -a did not return JSON: {exc}") from exc
    if not isinstance(data, list):
        raise ToolError("litar -a did not return a JSON array")
    try:
        return [BlockRecord.model_validate(item) for item in data]
    except ValidationError as exc:
        raise ToolError(f"litar -a returned an unexpected block: {exc}") from exc


def _fresh_blocks(path: str, labels: list[str], module: str, chunk: str) -> list[BlockRecord]:
    reference = encode_reference(labels, module, chunk)
    return _load_blocks("-l", reference, path)


mcp = MCPServer("litar", instructions=INSTRUCTIONS)


@mcp.tool(title="List modules", annotations=_READ_ONLY)
def list_modules(
    archive: Annotated[str, Field(description="Path of the litar archive.")],
) -> list[ModuleInfo]:
    """Fetch every module and its description.

    The description is the evaluated summary chunk: litar -m, then litar -p
    of summary or Module@/summary. References in the summary are expanded
    and its filters run. A module with no summary has a null description.
    Call blocks_applied on Module@/summary for the summary's own blocks.
    """

    with _LOCK:
        _require_archive(archive)
        listed = require_litar("-m", archive)
        # litar ends each name with LF. splitlines() would also split on
        # Unicode line breaks that can occur inside a name.
        raw = listed.stdout.decode("utf-8", "replace")
        if raw == "":
            modules: list[str] = []
        else:
            if raw.endswith("\n"):
                raw = raw[:-1]
            modules = raw.split("\n")
        found: list[ModuleInfo] = []
        for name in modules:
            evaluated = run_litar("-p", _summary_expression(name), archive)
            if evaluated.returncode == 0:
                found.append(
                    ModuleInfo(
                        module=name,
                        description=evaluated.stdout.decode("utf-8", "replace"),
                    )
                )
                continue
            message = evaluated.stderr.decode("utf-8", "replace")
            if _missing_summary(message, name):
                found.append(ModuleInfo(module=name, description=None))
                continue
            found.append(ModuleInfo(module=name, description=None, error=message.strip()))
        return found


@mcp.tool(title="Blocks applied to a chunk", annotations=_READ_ONLY)
def blocks_applied(
    archive: Annotated[str, Field(description="Path of the litar archive.")],
    chunk_reference: Annotated[
        str,
        Field(
            description="Chunk reference in litar syntax, without a closing @. "
            "Example: label@:module 1@/chunk. Write @ in a name as @@."
        ),
    ],
) -> list[BlockRecord]:
    """Check the blocks applied to a chunk reference.

    This is litar -a -l. The blocks are the ones -p would apply, in archive
    order, including less specialized blocks on the same specialization path.
    A later @= drops earlier blocks from the expanded text; blocks_effective
    returns the blocks that remain. file and line-range locate the block for
    modify_block and delete_block. A reference with no module matches that
    chunk in every module. An undefined reference is an error.
    """

    with _LOCK:
        _require_archive(archive)
        return _load_blocks("-l", chunk_reference, archive)


@mcp.tool(title="Effective blocks of a chunk", annotations=_READ_ONLY)
def blocks_effective(
    archive: Annotated[str, Field(description="Path of the litar archive.")],
    chunk_reference: Annotated[
        str,
        Field(
            description="Chunk reference in litar syntax, without a closing @. "
            "Example: label@:module 1@/chunk. Write @ in a name as @@."
        ),
    ],
) -> list[BlockRecord]:
    """Check the effective blocks of a chunk reference.

    This is litar -a -e. The blocks are the ones whose text remains after
    the chain is applied in archive order. A later @= drops every earlier
    block on that chain. @+ appends. Less specialized blocks stay when no
    later set drops them. A reference with no module matches that chunk in
    every module, and a set in one module does not drop a block in another.
    file and line-range locate the block for modify_block and delete_block.
    An undefined reference is an error.
    """

    with _LOCK:
        _require_archive(archive)
        return _load_blocks("-e", chunk_reference, archive)


@mcp.tool(title="Blocks that depend on a chunk", annotations=_READ_ONLY)
def blocks_depending(
    archive: Annotated[str, Field(description="Path of the litar archive.")],
    chunk_reference: Annotated[
        str,
        Field(
            description="Chunk reference in litar syntax, without a closing @. "
            "A use matches when it is this reference or a specialization of it."
        ),
    ],
) -> list[BlockRecord]:
    """Check the blocks that depend on a chunk reference.

    This is litar -a -d. A block depends on the reference when its content
    contains that reference, or a specialization of it, or a filter names it.
    The search is not transitive. An unknown reference with no users returns
    an empty list.
    """

    with _LOCK:
        _require_archive(archive)
        return _load_blocks("-d", chunk_reference, archive)


@mcp.tool(title="Evaluate an expression", annotations=_READ_ONLY)
def evaluate_expression(
    archive: Annotated[str, Field(description="Path of the litar archive.")],
    expression: Annotated[
        str,
        Field(
            description="Expression in litar syntax, without a closing @. "
            "Filters are @|name after the chunk. Example: module name@/chunk@|filter."
        ),
    ],
) -> Evaluation:
    """Evaluate an expression and return its content.

    This is litar -p. The content is the expanded chunk after its filters
    run. -a does not change this text. A missing chunk, a cycle, or a failed
    filter is an error and yields no content.
    """

    with _LOCK:
        if not expression:
            raise ToolError("expression is empty")
        _require_archive(archive)
        proc = require_litar("-p", expression, archive)
        return Evaluation(
            content=proc.stdout.decode("utf-8", "replace"),
            stderr=proc.stderr.decode("utf-8", "replace"),
        )


@mcp.tool(title="Modify a block", annotations=_OVERWRITE)
def modify_block(
    file: Annotated[
        str,
        Field(description="file from the block listing. May be an included archive."),
    ],
    start_line: Annotated[int, Field(ge=1, description="First line of line-range, inclusive.")],
    end_line: Annotated[int, Field(ge=1, description="Last line of line-range, inclusive.")],
    chunk: Annotated[str, Field(description="Chunk name, decoded.")],
    content: Annotated[
        str,
        Field(
            description="Raw block body. Keep @@ for a literal @. "
            "Do not include the layout newline after @= or @+."
        ),
    ],
    comment: Annotated[
        str,
        Field(description="Comment before the block, without leading blank lines."),
    ] = "",
    labels: Annotated[list[str] | None, Field(description="Labels stored on the block.")] = None,
    module: Annotated[
        str,
        Field(description="Module the block extends. Empty is the anonymous module."),
    ] = "",
    type: Annotated[
        Literal["set", "extend"],
        Field(description="set writes @=. extend writes @+."),
    ] = "set",
    filters: Annotated[
        list[Filter] | None,
        Field(description="Block filters, in order. Same objects the listing returns."),
    ] = None,
) -> EditResult:
    """Modify a block in place.

    The span is the line-range from blocks_applied, blocks_effective, or blocks_depending.
    The other fields replace that span. A blank line that the listing
    includes only because the comment is empty is kept. Names are decoded;
    the archive text escapes @ as @@. After the write, litar parses the file,
    and sed restores the previous text if the file no longer parses.
    """

    with _LOCK:
        _require_archive(file)
        _require_span(file, start_line, end_line)
        block_labels = list(labels or [])
        body = _anchor_blank(file, start_line, comment) + format_block(
            comment,
            block_labels,
            module,
            chunk,
            type,
            content,
            filters,
        )

        def mutate() -> None:
            replace_lines(file, start_line, end_line, body)

        _edit(file, mutate)
        written = _text_lines(body)
        return EditResult(
            file=file,
            start_line=start_line,
            end_line=start_line + written - 1,
            blocks=_fresh_blocks(file, block_labels, module, chunk),
        )


@mcp.tool(title="Delete a block", annotations=_OVERWRITE)
def delete_block(
    file: Annotated[str, Field(description="file from the block listing.")],
    start_line: Annotated[int, Field(ge=1, description="First line of line-range, inclusive.")],
    end_line: Annotated[int, Field(ge=1, description="Last line of line-range, inclusive.")],
) -> EditResult:
    """Delete the block that occupies this inclusive line range.

    The range is the line-range from a listing. A preceding @- is outside
    that range and stays. sed restores the file if it no longer parses.
    """

    with _LOCK:
        _require_archive(file)
        _require_span(file, start_line, end_line)

        def mutate() -> None:
            delete_lines(file, start_line, end_line)

        _edit(file, mutate)
        return EditResult(file=file, start_line=start_line, end_line=end_line)


@mcp.tool(title="Append a block", annotations=_APPEND)
def append_block(
    archive: Annotated[str, Field(description="Archive that receives the block.")],
    chunk: Annotated[str, Field(description="Chunk name, decoded.")],
    content: Annotated[
        str,
        Field(description="Raw block body. Keep @@ for a literal @."),
    ] = "",
    comment: Annotated[str, Field(description="Comment placed before the block header.")] = "",
    labels: Annotated[list[str] | None, Field(description="Labels stored on the block.")] = None,
    module: Annotated[
        str,
        Field(description="Module the block extends. Empty is the anonymous module."),
    ] = "",
    type: Annotated[
        Literal["set", "extend"],
        Field(description="set writes @=. extend writes @+."),
    ] = "set",
    filters: Annotated[list[Filter] | None, Field(description="Block filters, in order.")] = None,
) -> EditResult:
    """Append a block at the end of the archive.

    A @- selection for the block's module is written first, then a blank
    line, then the block. The selection is not part of the block's later
    line-range. The returned line span covers everything written, including
    that selection. blocks lists what litar -a -l now applies to the chunk.
    """

    with _LOCK:
        _require_archive(archive)
        block_labels = list(labels or [])
        block = format_block(comment, block_labels, module, chunk, type, content, filters)
        module_name = _clean_name("module", module, True)
        text = _module_selection(module_name) + "\n" + block
        before = line_count(archive)

        def mutate() -> None:
            append_lines(archive, text)

        _edit(archive, mutate)
        written = _text_lines(text)
        return EditResult(
            file=archive,
            start_line=before + 1,
            end_line=before + written,
            blocks=_fresh_blocks(archive, block_labels, module_name, chunk),
        )


def main() -> None:
    mcp.run()


if __name__ == "__main__":
    main()
