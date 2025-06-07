package cli

import (
	"github.com/eislab-cps/synctree/pkg/crdt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

func init() {
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(exportCmd)

	importCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	importCmd.MarkFlagRequired("prvkey")
	importCmd.Flags().StringVarP(&JSONFile, "json", "", "", "JSON file to import")
	importCmd.MarkFlagRequired("json")
	importCmd.Flags().StringVarP(&CRDTFile, "crdt", "", "", "File to store imported data")
	importCmd.MarkFlagRequired("crdt")
	importCmd.Flags().BoolVarP(&PrintJSON, "print", "p", false, "Print JSON to stdout")

	exportCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	exportCmd.MarkFlagRequired("prvkey")
	exportCmd.Flags().StringVarP(&JSONFile, "json", "", "", "JSON file to import")
	exportCmd.MarkFlagRequired("json")
	exportCmd.Flags().StringVarP(&CRDTFile, "crdt", "", "", "File to store imported data")
	exportCmd.MarkFlagRequired("crdt")
	exportCmd.Flags().BoolVarP(&PrintJSON, "print", "p", false, "Print JSON to stdout")
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a JSON file to CRDT SyncTree",
	Long:  "Import a JSON file to CRDT SyncTree",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
			"json": JSONFile,
			"crdt": CRDTFile,
		}).Info("Importing JSON file to CRDT SyncTree")

		c, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		jsonData, err := os.ReadFile(JSONFile)
		CheckError(err)

		_, err = c.ImportJSON(jsonData, PrvKey)
		CheckError(err)

		savedData, err := c.Save()
		CheckError(err)

		err = os.WriteFile(CRDTFile, savedData, 0644)
		CheckError(err)

		if PrintJSON {
			log.Info(string(jsonData))
		}
	},
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export CRDT SyncTree to a JSON file",
	Long:  "Export CRDT SyncTree to a JSON file",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
			"json": JSONFile,
			"crdt": CRDTFile,
		}).Info("Exporting CRDT SyncTree to JSON")

		c, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		crdtData, err := os.ReadFile(CRDTFile)
		CheckError(err)

		err = c.Load(crdtData)
		CheckError(err)

		jsonData, err := c.ExportJSON()
		CheckError(err)
		err = os.WriteFile(JSONFile, jsonData, 0644)
		CheckError(err)

		if PrintJSON {
			log.Info(string(jsonData))
		}
	},
}
