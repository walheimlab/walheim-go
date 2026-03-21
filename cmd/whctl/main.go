package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/walheimlab/walheim-go/internal/version"
)

func main() {
	root := buildRootCmd()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	var (
		contextFlag  string
		whconfigFlag string
		outputFlag   string // "table" | "json"
		quietFlag    bool
	)

	root := &cobra.Command{
		Use:   "whctl",
		Short: "Walheim homelab controller — kubectl-style CLI for self-hosted apps",
		Long: `whctl manages homelab environments: namespaces, apps, secrets, and configmaps.

Global flags apply to every command. Set WHCONFIG env var to override config file path.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	root.PersistentFlags().StringVar(&contextFlag, "context", "", "Override active context")
	root.PersistentFlags().StringVar(&whconfigFlag, "whconfig", "", "Alternate config file path (default: ~/.walheim/config or $WHCONFIG)")
	root.PersistentFlags().StringVarP(&outputFlag, "output", "o", "table", "Output format: table|json")
	root.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Bare output (one item per line, no headers)")

	// version subcommand
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show whctl version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("whctl version %s (commit: %s, built: %s)\n",
				version.Version, version.GitCommit, version.BuildDate)
			return nil
		},
	})

	return root
}
