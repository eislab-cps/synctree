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
	rootCmd.AddCommand(setLiteralCmd)
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(printCmd)
	rootCmd.AddCommand(verifyCmd)

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

	setLiteralCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	setLiteralCmd.MarkFlagRequired("prvkey")
	setLiteralCmd.Flags().StringVarP(&CRDTFile, "crdt", "", "", "File to store imported data")
	setLiteralCmd.MarkFlagRequired("crdt")
	setLiteralCmd.Flags().StringVarP(&NodePath, "path", "", "", "Path to the node in the CRDT SyncTree")
	setLiteralCmd.MarkFlagRequired("path")
	setLiteralCmd.Flags().StringVarP(&LiteralValue, "value", "", "", "String literal value to set")
	setLiteralCmd.MarkFlagRequired("value")
	setLiteralCmd.Flags().BoolVarP(&PrintJSON, "print", "p", false, "Print JSON to stdout")

	mergeCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	mergeCmd.MarkFlagRequired("prvkey")
	mergeCmd.Flags().StringVarP(&CRDTFileIn1, "crdt1", "", "", "First CRDT file to merge")
	mergeCmd.MarkFlagRequired("crdt1")
	mergeCmd.Flags().StringVarP(&CRDTFileIn2, "crdt2", "", "", "Second CRDT file to merge")
	mergeCmd.MarkFlagRequired("crdt2")
	mergeCmd.Flags().StringVarP(&CRDTFileOut, "crdtout", "", "", "Output CRDT file after merge")
	mergeCmd.MarkFlagRequired("crdtout")
	mergeCmd.Flags().BoolVarP(&PrintJSON, "print", "p", false, "Print JSON to stdout")

	printCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	printCmd.MarkFlagRequired("prvkey")
	printCmd.Flags().StringVarP(&CRDTFile, "crdt", "", "", "File to store imported data")
	printCmd.MarkFlagRequired("crdt")

	verifyCmd.Flags().StringVarP(&PrvKey, "prvkey", "", "", "Private key")
	verifyCmd.MarkFlagRequired("prvkey")
	verifyCmd.Flags().StringVarP(&CRDTFile, "crdt", "", "", "File to verify integrity of the CRDT SyncTree")
	verifyCmd.MarkFlagRequired("crdt")
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

var setLiteralCmd = &cobra.Command{
	Use:   "set-literal",
	Short: "Set a string literal value in CRDT SyncTree",
	Long:  "Set a string literal value in the CRDT SyncTree",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
			"json":  JSONFile,
			"crdt":  CRDTFile,
			"path":  NodePath,
			"value": LiteralValue,
		}).Info("Exporting CRDT SyncTree to JSON")

		c, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		crdtData, err := os.ReadFile(CRDTFile)
		CheckError(err)

		err = c.Load(crdtData)
		CheckError(err)

		node, err := c.GetNodeByPath(NodePath)
		CheckError(err)

		err = node.SetLiteral(LiteralValue, PrvKey)
		CheckError(err)

		savedData, err := c.Save()
		CheckError(err)
		err = os.WriteFile(CRDTFile, savedData, 0644)
		CheckError(err)

		jsonData, err := c.ExportJSON()
		CheckError(err)
		if PrintJSON {
			log.Info(string(jsonData))
		}
	},
}

var mergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge two CRDT SyncTree files",
	Long:  "Merge two CRDT SyncTree files into one",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
			"crdt1":   CRDTFileIn1,
			"crdt2":   CRDTFileIn2,
			"crdtout": CRDTFileOut,
		}).Info("Merging two CRDT SyncTree files")

		c1, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		data1, err := os.ReadFile(CRDTFileIn1)
		CheckError(err)

		err = c1.Load(data1)
		CheckError(err)

		c2, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		data2, err := os.ReadFile(CRDTFileIn2)
		CheckError(err)

		err = c2.Load(data2)
		CheckError(err)

		err = c1.Merge(c2, PrvKey)
		CheckError(err)

		savedData, err := c1.Save()
		CheckError(err)

		err = os.WriteFile(CRDTFileOut, savedData, 0644)
		CheckError(err)

		if PrintJSON {
			jsonData, err := c1.ExportJSON()
			CheckError(err)
			log.Info(string(jsonData))
		}
	},
}

var printCmd = &cobra.Command{
	Use:   "print",
	Short: "Print CRDT SyncTree as JSON",
	Long:  "Print the current state of the CRDT SyncTree as JSON",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
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
		log.Info(string(jsonData))
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify CRDT SyncTree integrity",
	Long:  "Verify the integrity of the CRDT SyncTree",
	Run: func(cmd *cobra.Command, args []string) {
		log.WithFields(log.Fields{
			"crdt": CRDTFile,
		}).Info("Exporting CRDT SyncTree to JSON")

		c, err := crdt.NewSecureTree(PrvKey)
		CheckError(err)

		crdtData, err := os.ReadFile(CRDTFile)
		CheckError(err)

		err = c.Load(crdtData)
		CheckError(err)

		err = c.VerifyTree()
		CheckError(err)

		log.Info("CRDT SyncTree integrity verified successfully")
	},
}
