# About the project

This project proposes a system named litar, which is defined in [design/litar.md](design/litar.md).
[design/stages.md](design/stages.md) describes the separated stages of the development.

# Directories

- design: design files describing what this project should be from human's view;
- skills: skills for an agent to deal with this project;
- examples: some examples written in litar format;
- src: the generated source files;
- tests: tests for the features implemented so far.

# Build and test

`make` compiles `src/stage0.c` into `./litar`.
`make test` compiles it, then runs the tests under `tests/`.
`make clean` removes the binary.

