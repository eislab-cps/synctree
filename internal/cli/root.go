package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const TimeLayout = "2006-01-02 15:04:05"

var Verbose bool
var PrvKey string
var JSONFile string
var CRDTFile string
var PrintJSON bool
var NodePath string
var LiteralValue string
var CRDTFileIn1 string
var CRDTFileIn2 string
var CRDTFileOut string

func init() {
	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "verbose output")
}

var rootCmd = &cobra.Command{
	Use:   "synctree",
	Short: "synctree",
	Long:  "synctree",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
