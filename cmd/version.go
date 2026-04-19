package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd prints release/build metadata for traceable distribution.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show relayctl build version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(buildInfoString())
		},
	}
}
