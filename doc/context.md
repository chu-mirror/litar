# Context

A composition holds far more than one task needs.
This document describes how work on a composition is divided among workers
so that each worker holds only the context of its own task,
how the division is recorded in git, and what litar adds to support it.
Nothing here is implemented yet; the additions are listed at the end,
and each goes into [implementation.md](implementation.md) and [syntax.md](syntax.md)
when it is implemented.

# Model

A **worker** is whoever does a task: a human or an agent. Every worker follows the same protocol,
so a task can be handed to either, at any depth.

1. A worker receives a task on a composition, the **origin**.
2. It measures the scale of the task.
3. If the task is too big for it, it divides the task into sub-tasks by an outline
   (see [Dividing by outline](#dividing-by-outline)).
4. For each sub-task it selects the context and opens a **task composition**:
   the origin checked out on a task branch, with a scope, a description, and acceptance checks.
5. It judges the ability each sub-task needs and assigns it to a suitable worker.
6. It waits. When a sub-task is finished, it merges the task branch into the origin,
   checks the integrity of the merged origin itself, and commits the merge.
   It repeats until no sub-task is left.

A worker that receives a task composition treats it as its origin,
so the protocol nests: a sub-task may be divided again on its own task branch.

A task the worker does itself needs no task composition.
It may still use a task node, as described in [style.md](../design/style.md):
nodes stay a place for blocks that contradict each other or whose placement is undecided.
A task composition is the unit of delegation.

What a worker keeps in its own context is its task, the scopes it handed out,
and the reports that come back. It never needs the blocks its sub-workers read.

# Sub-tasks only add blocks

**A sub-task never changes or deletes a block that exists in the origin**, its comment included.
It applies new blocks, and may change or delete only the blocks it applied itself.

- A merge is then a union: the blocks of the origin, with the blocks each task added.
  Two tasks cannot both change one block, so they cannot conflict over one.
- What a sub-worker produced is exactly its new blocks;
  the merging worker reviews those and nothing else.
- Nothing a sibling relies on disappears under it.

Only the worker that owns the origin changes or deletes its blocks.
To delegate a change, it first deletes the blocks to be replaced in the origin, commits that,
and delegates the chunk, now undefined, as a task that writes it anew.
The deletion commit records what was replaced and why.

# Tasks in git

The origin is a branch; every other part of the workflow is a commit or a branch.

| Workflow | git |
| --- | --- |
| origin | a branch: `main` at the top, a task branch for a nested origin |
| outline and acceptance checks | a commit on the origin |
| task composition | a branch `task/<name>`, checked out in its own worktree |
| task description and scope | an empty first commit on the task branch, the **task commit**, with trailers |
| the work | commits on the task branch that add blocks |
| report | the last commit of the task branch |
| merge and integrity check | `git merge --no-ff --no-commit`, the check, then the merge commit |
| dependency | ancestry: the task branch starts from the merge of every task it needs |

## Dependencies

A task that needs the result of another starts from the merge of that task;
a task that needs nothing new starts from the outline commit.
**Task B depends on task A exactly when the merge of A is an ancestor of the task commit of B**
(`git merge-base --is-ancestor`), so the dependency graph of the tasks is the commit graph.

An outline that divides `apply 'text' to 'n'` into three parts, where storing needs the parse:

| Commit | Parents | Message |
| --- | --- | --- |
| O | A | Outline: apply 'text' to 'n' |
| p1 | O | Task: parse the archive of 'n' followed by 'text' into 'arch' |
| p2 | p1 | Parse the joined archive |
| t1 | O | Task: test applying a block |
| t2 | t1 | Add the cases of apply |
| Mp | O, p2 | Merge task parse |
| s1 | **Mp** | Task: store 'arch' as the own blocks of 'n' |
| s2 | s1 | Store the compacted archive |
| Mt | Mp, t2 | Merge task tests |
| Ms | Mt, s2 | Merge task store |

`task/parse` and `task/tests` branch from O and run at the same time;
`task/store` branches from Mp, so it sees the parse and is recorded as depending on it.
A task that needs several others starts from a commit where all of them are merged.

On a task branch, the first-parent chain from its head stays on that branch,
through the merges of its own sub-tasks, down to its task commit.
So the task of any worktree is the nearest commit with a `Litar-Task` trailer on that chain,
and the top origin, with none, has no scope.

## Task commit

The task commit is empty. Its subject and body are the task description;
its trailers are the name, the scope, and the acceptance checks, one value per trailer.

```
Task: parse the archive of 'n' followed by 'text' into 'arch'

Join the text to the archive of the node and parse the result;
the store is a separate task.

Litar-Task: parse
Litar-Writable: Project@/litar@/Composer@/Apply@/parse the archive of 'n' followed by 'text' into 'arch'
Litar-Writable: Project@/litar@/Composer@/Apply@/Parse
Litar-Visible: Project@/litar@/Composer@/apply 'text' to 'n'
Litar-Visible: Project@/litar@/Parser
Litar-Acceptance: Project@/litar@/Tests@/Cases@/apply
```

Opening the task composition:

```
git switch -c task/parse O
git commit --allow-empty -F task-message
git worktree add .tasks/parse task/parse
```

The worktree holds the whole repository at that commit:
its grounds and mapped directories inside the repository are its own,
so a build in a task writes into the task's `bin`, not the origin's.
A ground is looked for in the composition's own repository before the library path
(see [Additions](#additions)), so a task composition loads the grounds of its worktree.
`.tasks/` is ignored by git.

# Scope

The scope is three sets of blocks. Each `Litar-Visible` and `Litar-Writable` value is one of:

- a module path: every chunk below it;
- a chunk reference, with or without labels: the blocks applied to it;
- an item: the blocks carrying it;
- a file path, or a directory ending in `/`: the blocks of those files.

| Set | Rule |
| --- | --- |
| **evaluated** | always every block of the composition and its ground |
| **visible** | the blocks that the visible and writable values name |
| **writable** | the chunks that the writable values name: new blocks may be applied to them |

The evaluated set is never narrowed. litar is global: any block may extend any chunk with `@+`,
an `@=` replaces earlier blocks, and a reference that no block defines expands to empty.
Hiding blocks from evaluation would give the worker a different program from the one it is changing.

Listings, trees, and searches return only visible blocks;
a tree shows a branch outside the visible set as one line with its count of chunks.
A block outside the visible set can still be read by its ref;
the read succeeds and is recorded, and the worker lists it in its report.
Recorded reads tell the delegating worker that it chose the context badly.

In a task composition the server refuses:

- a block applied to a chunk outside the writable set;
- an override or deletion of a block that exists at the task commit;
- renames, file moves, mappings, and node changes, which change names other tasks refer to.

# Dividing by outline

Before dividing, the worker commits an **outline** to the origin:
a block whose content refers to chunks that no block defines yet,
with the objects each part works on quoted and declared above it, as in [style.md](../design/style.md).

```
@<Project@/litar@/Composer@/apply 'text' to 'n'@=
var arch *Archive
@<.@/Apply@/parse the archive of 'n' followed by 'text' into 'arch'@>
@<.@/Apply@/store 'arch' as the own blocks of 'n'@>
@
```

Each undefined chunk is one sub-task:

- its writable set is that chunk, and the submodule for its helpers;
- its visible set is the outline block and what the quoted objects need;
- its interface is the quoted objects, fixed before anyone starts.

So the writable sets of siblings are disjoint by construction,
and the split is in the history of the origin, where every later reader sees it.

The same commit holds each task's acceptance check,
for example a case under `Project@/<project>@/Tests@/Cases`.
The check exists at the task commit, so the sub-worker sees it and cannot change it.

# Measuring a task

The scale of a task is the size of its scope, which `scope://` reports:
the visible blocks and their tokens (estimated as characters divided by 4),
and the writable chunks.

- A task is small enough for a worker when its visible tokens fit in a quarter of that worker's
  context budget, leaving room for reading, writing, and checking.
- A task that writes a few blocks of one chunk is not divided further:
  opening, merging, and checking a task composition costs more than doing it.

The ability a task needs follows from what it adds: a new decision or a new interface
(an outline, a data structure, an algorithm) needs a stronger worker than filling a chunk
whose interface and acceptance check are already written.

# Reading a scope

Every route into a composition returns whole blocks today,
so the context a worker needs grows with the composition, not with the task.
Two changes make reading proportional to the task.

**Short listings.** Every `blocks://` resource and `search_blocks` return short entries,
`{ref, node, header, summary}`, where `summary` is the first line of the block's comment.
`block://{composition}/{node}/{ref}` returns one block in full.
A worker skims the list, then reads only what it will touch;
refs are stable, so the second read costs nothing extra.
On the node `Litar` the headers and refs of all blocks are 89k characters against 618k for the blocks.

**Focus.** `focus://{composition}/{node}/{ref}`, for the ref of a chunk or an item, returns:

- the effective blocks of the chunk, or the blocks carrying the item, in full;
- every chunk they refer to, as its name and description;
- the blocks that depend on the chunk or carry its items, as short entries.

That is the working set for changing one thing, in one read.

# Finishing a task

The sub-worker ends by committing its report as the last commit of the task branch.
Its message summarizes what was done; its trailers give the result and the reads outside the scope.

```
Parse the joined archive

The text is joined after a line feed when the archive ends with the "@" of its last block.

Litar-Result: done
Litar-Outside: 85b56956eb
```

| Result | Meaning |
| --- | --- |
| `done` | the acceptance checks pass in the task composition |
| `divide` | the task is too big; the message proposes an outline |
| `context` | the scope is insufficient; the message names what is missing |
| `reject` | the task is wrong, or needs a change to an existing block; the message says why |

The decisions are in the comments of the new blocks, so the report does not repeat them.
`diff://{composition}` lists the blocks added since the task commit, one line each:

```
+ 001fddfe13 @<parse@:litar@:Language@/Go@/File@/import paths@+
+ 789413033e @<Project@/litar@/Composer@/Apply@/parse the archive of 'n' followed by 'text' into 'arch'@=
```

For `divide`, `context`, and `reject`, the delegating worker does not merge.
It changes the outline, the scope, or the origin, opens a new task whose task commit quotes
the report, and deletes the old branch, so the report stays in the history.

# Merging

The merging worker merges with git, and litar merges the archives.

```
git merge --no-ff --no-commit task/parse
```

`.gitattributes` sends every archive to the litar merge driver, which works on blocks, not lines:

- the result is the base blocks with the changes of the origin, then the blocks the origin added,
  then the blocks the task added, compacted as every edit is;
- it fails when the task changed or deleted a base block, when both sides added an `@=` block
  to the same chunk reference, or when a side does not parse.

Line merges would conflict whenever two tasks append to the end of one archive, or compaction places
their blocks after the same block, although their blocks are independent.

## Integrity check

The merging worker checks the merged origin before the merge commit exists:

1. the task branch changed nothing but archives (`git diff --name-only` against the merge base);
2. the acceptance checks of the task;
3. no reference left undefined that the outline introduced (`list://undefined`);
4. the files of every node under the nodes that received blocks, and the build and tests of the origin.

The server does not see a change git made to the archives until `import_composition` is called again.

If every check passes, `git commit` makes the merge commit, and the worker removes the worktree
and the branch (`git worktree remove`, `git branch -d`). If a check fails, `git merge --abort`
leaves the origin as it was, and the worker sends the task back with what failed.
The sub-worker continues on the same branch, where the blocks it added are still its own to change,
and commits a new report; the worker merges again.

The worker that owns the origin may change its blocks while tasks run. It avoids changing the
visible sets of running tasks; the merge driver accepts those changes, and the integrity check
catches what they break.

# Additions

Each is described in [implementation.md](implementation.md) when it is implemented.

**Command line**

| Option | Effect |
| --- | --- |
| `-m, --merge BASE OURS THEIRS` | the git merge driver: merge three versions of an archive by blocks into OURS; exit 1 on a conflict, listing it on stderr |

**Composer**

| Change | Effect |
| --- | --- |
| ground lookup | look for `NAME/composer.json` at the root of the git repository that holds the composition, before the current directory and the library path |

Without it, one process serving several worktrees, such as the MCP server of an agent session,
resolves every ground from its own working directory and `LITAR_LIBRARY`,
so every task composition would load the origin's grounds.

**Repository**

```
# .gitattributes
*.la merge=litar

# .gitignore
.tasks/

# git config
merge.litar.name = litar archives
merge.litar.driver = litar --merge %O %A %B
```

**MCP server**

| Kind | Name | Effect |
| --- | --- | --- |
| change | composition loading | in a git worktree, read the task commit of the head; its scope and acceptance checks apply to the composition |
| change | `apply_block`, `delete_block`, `move_block`, `rename`, `operate_files`, `map_file`, `create_node`, `delete_node` | in a task composition, refuse what [Scope](#scope) refuses |
| change | `blocks://…`, `search_blocks`, `tree://…` | short entries; only visible blocks in a task composition |
| resource | `block://{c}/{n}/{ref}` | one block in full |
| resource | `focus://{c}/{n}/{ref}` | the working set of a chunk or an item |
| resource | `scope://{c}` | the task, its scope, its size, and the recorded reads outside it |
| resource | `diff://{c}` | the blocks added since the task commit |
| resource | `list://undefined/{c}/{n}` | chunk references that no block defines |

Opening, merging, and closing a task are git commands, so a human and an agent run the same ones,
and litar needs no tool for them. The server reads git only to find the task commit
and the blocks at it.

The trailers need an entry in [syntax.md](syntax.md).
After the implementation, `litar-task` gains delegation (opening, assigning, and merging
task compositions), and the references describe the scope, the short listings, and the focus resource.

# In Claude Code

Claude Code runs the model with the features it has; one adaptation and two guards are needed.

| Model | Claude Code |
| --- | --- |
| delegating and merging worker | the main session, following `litar-task` |
| sub-worker | a sub-agent started in the background with the Agent tool |
| the ability a task needs | the `model` and `effort` of the sub-agent, or an agent definition in `.claude/agents/` |
| opening a task composition | the git commands of [Task commit](#task-commit), run through Bash |
| scope and refusals | the litar MCP server of the session, which imports the worktree's composition by its path |
| waiting | the notification the main session gets when a background sub-agent finishes |
| sending a task back | `SendMessage` to the same sub-agent, which keeps its context, on the same branch |
| report | the last commit of the task branch, read with `git log -1 --format=%B task/<name>` |
| merge and integrity check | the git commands of [Merging](#merging) through Bash, `import_composition`, then the checks |
| human worker | works in the worktree and commits the report; the main session watches for the `Litar-Result` trailer with the Monitor tool |

The Agent tool's own worktree isolation is not used: it picks the branch and its start,
and a task branch must start at a chosen commit with the task commit on top.

## Flat orchestration

A sub-agent cannot start sub-agents of its own, so the protocol cannot nest in sub-agents.
A sub-agent whose task is too big reports `divide` with an outline;
the main session commits the outline on that task branch, opens the sub-tasks from it,
and merges them into it before merging the task branch into its origin.
The commit graph is nested exactly as in [Dependencies](#dependencies); only the waiting is done
by one session. Each sub-agent still holds only its own task.

Nesting in processes is possible: a worker runs `claude -p` in a worktree, with an MCP server
of its own started there, and resumes it with `--resume` instead of `SendMessage`.

## One server for every worktree

The sub-agents share the session's `litar -a`, started in the origin's checkout.
Its lock is released while an expression is evaluated, so the builds of several tasks run at once.
Grounds are resolved per composition by the ground lookup of [Additions](#additions);
the binary is the origin's, which is right unless a task changes litar itself,
whose build and tests then run the task's own `bin/litar` from its worktree.

## Confining a sub-agent

Claude Code does not confine a sub-agent to a scope; litar does.

- The agent definition `.claude/agents/litar-worker.md` gives a sub-worker the litar MCP tools,
  Read, and Bash, with permission rules that allow only the git commands of a worker:
  `git add`, `git commit`, `git status`, `git log`, and `git diff`.
  It says what a sub-worker does: work in its task composition, add blocks only,
  and end with a report commit. It links to `agent/skills/references/` rather than repeating them.
- A block changed through a file edit instead of the server is still refused at the merge:
  the merge driver rejects a task branch that changed or deleted an existing block.

Delegation goes into `litar-task`, so the skills stay three.

# Open questions

- **Grounds outside the repository.** A ground found on the library path outside the repository
  is shared by every worktree; a task whose work belongs there reports `reject`,
  and the owner of that ground opens the task in its repository.
- **Mappings outside the repository.** A mapping whose origin is outside the repository is shared
  by every worktree, so a task that writes there affects its siblings; such mappings should be
  read-only in tasks.
