// internal/cli/fetch_modelmetadata.go
package agon

import (
	"fmt"

	"github.com/mwiater/agon/internal/models"
	"github.com/spf13/cobra"
)

// fetchModelMetadataCmd implements 'fetch modelmetadata', which fetches model IDs
// from configured host URLs and prints the normalized metadata.
var fetchModelMetadataCmd = &cobra.Command{
	Use:   "modelmetadata",
	Short: "Fetch model metadata from configured hosts",
	Long:  "The 'modelmetadata' subcommand fetches model IDs from configured host URLs and prints the normalized metadata.",
	Run: func(cmd *cobra.Command, args []string) {
		if len(fetchModelMetadataURLs) == 0 {
			fmt.Println("missing required --urls flag")
			return
		}
		modelNames := models.FetchEndpointModelNames(fetchModelMetadataURLs)
		models.FetchModelMetadata(modelNames)
	},
}

var fetchModelMetadataURLs []string

func init() {
	fetchCmd.AddCommand(fetchModelMetadataCmd)
	fetchModelMetadataCmd.Flags().StringSliceVar(&fetchModelMetadataURLs, "endpoints", nil, "comma-separated list of base URLs to query")
	_ = fetchModelMetadataCmd.MarkFlagRequired("endpoints")
}
