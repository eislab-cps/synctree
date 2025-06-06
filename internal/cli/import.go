package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(TalkCmd)
}

var TalkCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a JSON file",
	Long:  "Import a JSON file",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Importing a JSON file...")
	},
}
