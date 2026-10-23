# land

cld's notes for the `land` skill.

## Checks

Every check green: `lint`, `linux`, `linux-oldest`, `blesh` and `macos`. A one-off `macos` flake,
such as `fork failed: Device not configured`, gets one rerun, `gh run rerun RUN --failed`. Stop on
any other failure, and on a second flake.

## Release

You tag the release yourself once the stack has landed, on main's head, and push the tag:

```sh
git tag -a vX.Y.Z -m vX.Y.Z origin/main
git push origin vX.Y.Z
```

The workflow `release` runs `make docker-check`, then publishes a binary per platform, `cld.sha256`
and `install.sh`.
