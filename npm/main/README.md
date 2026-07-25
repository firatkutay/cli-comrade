# cli-comrade

Cross-platform AI CLI companion for the terminal (TR/EN). Talk to it in
natural language; it analyzes errors, proposes commands, and runs them
under a configurable safety mode (`auto`/`ask`/`info`).

This package installs a small dispatcher (`bin/comrade.js`) that resolves
and runs the prebuilt `comrade` binary shipped for your platform as an
`optionalDependency` -- no Go toolchain or build step required.

## Updating

Installed this way, `comrade upgrade` refuses to self-update the binary
in place -- doing so would desync your package manager's own recorded
installed version from what's actually on disk, and its next update
command would silently revert it. Update through the same package
manager you installed it with instead:

```sh
npm update -g cli-comrade
```

(`pnpm update -g cli-comrade` / `yarn global upgrade cli-comrade` for
pnpm/yarn installs.) `comrade upgrade --check` still works normally --
it only reports whether a newer version exists, and never refuses.

Full documentation, other install channels, and source:
https://github.com/firatkutay/cli-comrade
