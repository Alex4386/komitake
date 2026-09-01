package main

import (
	"github.com/spf13/cobra"
)

// Cobra generates a completion command automatically, but its default help text
// does not explain installation. This replaces it with per-shell instructions.
func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "Generate a shell completion script",
		Long: `Generate a shell completion script for komitake.

Completion includes device serials for -s, queried live from the daemon.

Bash:
  komitake completion bash | sudo tee /etc/bash_completion.d/komitake

Zsh:
  komitake completion zsh > "${fpath[1]}/_komitake"

Fish:
  komitake completion fish > ~/.config/fish/completions/komitake.fish`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			}
			return nil
		},
	}
	return cmd
}
