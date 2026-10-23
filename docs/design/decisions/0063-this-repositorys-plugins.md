# 63. This repository's plugins

Status: Accepted.

## Context

This repository kept its own skills and lint hooks for Claude Code in `.claude/`. The author's
repositories now share them through one plugin marketplace,
[zadykian/skills](https://github.com/zadykian/skills). A project enables a marketplace's plugins in
`.claude/settings.json`, through the keys `extraKnownMarketplaces` and `enabledPlugins`. cld writes
neither, as it leaves plugins to claude and the user
([decision 53](0053-project-and-user-settings.md)).

This amends decisions [19](0019-project-settings.md) and [28](0028-shared-project-settings.md),
which keep this repository's settings byte for byte what cld writes, but for their hooks.

## Decision

This repository's `.claude/settings.json` is what `setup config project --mcp goland
--permissions cld` writes, then the keys of its own Claude Code setup, which cld does not write.
They are `extraKnownMarketplaces` and `enabledPlugins`, which enable the plugins `flow`, `docs` and
`lint`, then any hooks. The tests compare the file up to `extraKnownMarketplaces`.

Not taken: `--permissions cld` writing the plugins, which would enable one author's marketplace in
each project that takes the set.

## Consequences

The skills and hooks that the plugins hold leave `.claude/`. The `adr` skill stays, being cld's
own. What this repository adds to a shared skill goes in `.claude/skill-notes/`, which the skill
reads first.
