// Package cli wires together cli-comrade's cobra command tree: the "comrade"
// root command and its subcommands (fix, explain, chat, config, init,
// history, plus the hidden "hook" group). In FAZ 0 the subcommands were
// unimplemented stubs; FAZ 1 replaced config's, FAZ 4 replaced init's
// (internal/cli/init.go) and added the hidden "hook record" entry point
// (internal/cli/hook.go) that shell hooks invoke — see internal/shellinit
// for the snippets themselves.
package cli
