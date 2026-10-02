# 7. JediTerm pin

Status: Accepted.

## Context

The JediTerm driver runs JediTerm's emulator headless, from the `jediterm-core` that JetBrains
publishes to its Maven repository ([testing](../testing.md),
[terminal findings](../findings/terminals.md)).

## Decision

JediTerm is pinned at 3.76, the latest published when the driver came in, and bumped
deliberately.

## Consequences

A new JediTerm release changes no test run until the pin moves. The bump is a change of its own,
as is that of the newest tmux the tests run on ([decision 6](0006-versions.md)).
