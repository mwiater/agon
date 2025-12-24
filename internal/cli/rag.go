package agon

import "github.com/spf13/cobra"

// ragCmd groups RAG-related CLI commands.
var ragCmd = &cobra.Command{
	Use:   "rag",
	Short: "RAG utilities",
}

func init() {
	rootCmd.AddCommand(ragCmd)
}
