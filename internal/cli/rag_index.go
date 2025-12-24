package agon

import (
	"context"

	"github.com/mwiater/agon/internal/rag"
	"github.com/spf13/cobra"
)

// ragIndexCmd builds the JSONL index from the RAG corpus.
var ragIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Build the RAG JSONL index",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return rag.BuildIndex(context.Background(), GetConfig())
	},
}

func init() {
	ragCmd.AddCommand(ragIndexCmd)
}
