// internal/commands/fetch_modelmetadata.go
package agon

import (
	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// fetchModelMetadataCmd implements 'fetch modelmetadata', which fetches model IDs
// from configured host URLs and prints the normalized metadata.
var fetchModelMetadataCmd = &cobra.Command{
	Use:   "modelmetadata",
	Short: "Fetch model metadata from configured hosts",
	Long:  "The 'modelmetadata' subcommand fetches model IDs from configured host URLs and prints the normalized metadata.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return models.FetchModelMetadataFromEndpoints(fetchModelMetadataURLs, fetchModelMetadataGPU)
	},
}

var fetchModelMetadataURLs []string
var fetchModelMetadataGPU string

func init() {
	fetchCmd.AddCommand(fetchModelMetadataCmd)
	fetchModelMetadataCmd.Flags().StringSliceVar(&fetchModelMetadataURLs, "endpoints", nil, "comma-separated list of base URLs to query")
	_ = fetchModelMetadataCmd.MarkFlagRequired("endpoints")
	fetchModelMetadataCmd.Flags().StringVar(&fetchModelMetadataGPU, "gpu", "", "GPU identifier used as a filename prefix")
	_ = fetchModelMetadataCmd.MarkFlagRequired("gpu")
}
