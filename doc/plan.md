# Research plan: litar and context engineering

Context engineering is about getting the smallest sufficient set of tokens
into the model. This plan examines whether a litar composition is a better
substrate for that than plain files.

The benefit is plausible but unproven, and the baseline to beat is strong.
Nothing below has been measured yet.

# Hypothesis

## Where litar could help

| litar feature | Context-engineering effect |
| --- | --- |
| Chunk, item, and file trees with names and descriptions | Progressive disclosure: read an outline first, drill into units of about 150 tokens instead of whole files |
| `blocks://depend`, `blocks://effective`, `blocks://refer` | The context for an edit is computed from a graph rather than guessed by grep |
| Order-irrelevant sequences | Write locality: adding an include needs only the chunk name |
| Decision comments before blocks | The reason for a decision lives beside the code and survives across sessions |
| Task nodes and digest | Task isolation and a structured form of compaction |
| Filters from a single specification | Fewer duplicated tokens and less drift between copies |

## Why it might not

- **Out-of-distribution format.** Models are trained on plain files,
  so the control characters may raise error rates and cost tokens.
- **Strong free baselines.** Grep, LSP, and tree-sitter repo maps give structure
  without any authoring cost.
- **Round trips.** Small chunks mean many tool calls, and ids change after every mutation,
  so trees must be fetched again.
- **Unverified metadata.** Names, descriptions, and `@~` items are written by hand or by an LLM;
  stale ones mislead.
- **Authoring and maintenance cost.** Porting a project and keeping the structure healthy
  costs tokens that must be counted against the savings.

# Core question

At equal task success, does an agent working on a litar composition use fewer tokens
than one working on plain files? Or, at an equal token budget, does it succeed more often?

# Stages

## 1. Instrument and build a corpus

- Log tokens, tool calls, and turns per session.
- Port 3 to 5 C or Go projects of 5k to 30k lines with the `litar-port` skill.
- Keep all three port stages (every file as one block, split into chunks, cross-referenced)
  as an ablation ladder.

## 2. Read-side experiment

- Ask localisation and comprehension questions with ground truth,
  such as "what must change to do Y".
- Compare five conditions: plain files with grep, plain files with a repo map or LSP,
  and litar at each of the three port stages.
- Measure accuracy, tokens loaded, and the share of loaded tokens that were relevant.

## 3. Write-side experiment

- Replay real commits that come with tests, taken from the histories of the ported projects.
- Measure pass rate, tokens, turns, the rate of malformed blocks, and regressions.

## 4. Style-rule ablations

- Vary the block length: 75, 150, 300, and 600 tokens.
  The figure of 150 in the style guide was chosen for humans, not for models.
- Toggle descriptive names against opaque ones, descriptions, cross-references,
  and decision comments.

## 5. Long-horizon experiment

- Run 10 to 20 dependent tasks on one code base across fresh sessions.
- Measure whether the structure decays, how often cross-references go stale,
  and whether decision comments can replace external memory.
- Compare a task node followed by a digest against editing in place.

## 6. Budget and model sweep

- Repeat the read-side and write-side experiments with a capped context and with a weaker model.
- The expectation is that structure helps most when context is scarce.

# Stop condition

If the cross-referenced stage does not beat plain files with a repo map
in the read-side experiment, the write-side experiment is unlikely to rescue it.
Rethink before investing further.

# Confounds to control

- Count the skill and reference documents as context cost.
- Count the porting cost.
- Keep test tasks out of anything the agent can read.

# Advice on the design

These come from reading the design files with context cost in mind.

- **Stable addressing.** References based on names would avoid fetching the trees again
  after each edit.
- **A context-closure resource.** One call that returns a block together with the names
  and descriptions of its dependencies down to a given depth, under a token budget.
- **Error mapping.** A way to map a line of a generated file back to its chunk,
  so that compiler errors lead somewhere.
- **Item checking.** Some verification that `@~` items still match the content of their block.

