// Package cli wires together cli-comrade's cobra command tree: the "comrade"
// root command and its subcommands (fix, explain, chat, config, init,
// history). In FAZ 0 the subcommands are unimplemented stubs; later phases
// replace each stub body with real behavior without changing the command
// tree shape.
package cli
