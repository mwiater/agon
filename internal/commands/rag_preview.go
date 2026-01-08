package agon

import (
	"context"

	"github.com/mwiater/agon/internal/rag"
	"github.com/spf13/cobra"
)

// ragPreviewCmd previews RAG retrieval and context assembly for a query.
var ragPreviewCmd = &cobra.Command{
	Use:   "preview <query>",
	Short: "Preview RAG retrieval and context assembly",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return rag.RunPreviewCommand(context.Background(), GetConfig(), args)
	},
}

func init() {
	ragCmd.AddCommand(ragPreviewCmd)
}
