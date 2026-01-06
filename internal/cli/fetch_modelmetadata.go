// internal/cli/fetch_modelmetadata.go
package agon

import (
	"fmt"
	"strings"

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
		if len(fetchModelMetadataURLs) == 0 {
			return fmt.Errorf("missing required --endpoints flag")
		}
		if strings.Contains(fetchModelMetadataGPU, " ") || strings.Contains(fetchModelMetadataGPU, "_") {
			return fmt.Errorf("--gpu value must not contain spaces or underscores")
		}
		modelNames := models.FetchEndpointModelNames(fetchModelMetadataURLs)
		models.FetchModelMetadata(modelNames, fetchModelMetadataGPU)
		return nil
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
