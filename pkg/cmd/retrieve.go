package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gptscript-ai/knowledge/pkg/datastore"
	flowconfig "github.com/gptscript-ai/knowledge/pkg/flows/config"
	vserr "github.com/gptscript-ai/knowledge/pkg/vectorstore/errors"
	"github.com/spf13/cobra"
	"log/slog"
)

type ClientRetrieve struct {
	Client
	Datasets []string `usage:"Target Dataset IDs" short:"d" default:"default" env:"KNOW_TARGET_DATASETS" name:"dataset"`
	Archive  string   `usage:"Path to the archive file"`
	ClientRetrieveOpts
	ClientFlowsConfig
}

type ClientRetrieveOpts struct {
	TopK     int      `usage:"Number of sources to retrieve" short:"k" default:"10"`
	Keywords []string `usage:"Keywords that retrieved documents must contain" short:"w" name:"keyword" env:"KNOW_RETRIEVE_KEYWORDS"`
}

func (s *ClientRetrieve) Customize(cmd *cobra.Command) {
	cmd.Use = "retrieve [--dataset <dataset-id>] <query>"
	cmd.Short = "Retrieve sources for a query from a dataset"
	cmd.Args = cobra.ExactArgs(1)
}

func (s *ClientRetrieve) Run(cmd *cobra.Command, args []string) error {
	c, err := s.getClient()
	if err != nil {
		return err
	}

	datasetIDs := s.Datasets
	if len(s.Datasets) == 0 {
		datasetIDs = []string{"default"}
	}
	query := args[0]

	retrieveOpts := datastore.RetrieveOpts{
		TopK:     s.TopK,
		Keywords: s.Keywords,
	}

	if s.FlowsFile != "" {
		slog.Debug("Loading retrieval flows from config", "flows_file", s.FlowsFile, "dataset", datasetIDs)
		flowCfg, err := flowconfig.FromFile(s.FlowsFile)
		if err != nil {
			return err
		}
		var flow *flowconfig.FlowConfigEntry
		if s.Flow != "" {
			flow, err = flowCfg.GetFlow(s.Flow)
			if err != nil {
				return err
			}
		} else {
			if len(datasetIDs) == 1 {
				flow, err = flowCfg.ForDataset(datasetIDs[0]) // get flow for the dataset
				if err != nil {
					return err
				}
			} else {
				flow, err = flowCfg.GetDefaultFlowConfigEntry()
				if err != nil {
					return err
				}
			}
		}

		if flow.Retrieval == nil {
			slog.Info("No retrieval config in assigned flow", "flows_file", s.FlowsFile, "dataset", datasetIDs)
		} else {
			rf, err := flow.Retrieval.AsRetrievalFlow()
			if err != nil {
				return err
			}
			retrieveOpts.RetrievalFlow = rf
			slog.Debug("Loaded retrieval flow from config", "flows_file", s.FlowsFile, "dataset", datasetIDs)
		}
	}

	retrievalResp, err := c.Retrieve(cmd.Context(), datasetIDs, query, retrieveOpts)
	if err != nil {
		// An empty collection is not a hard error - the LLM session can "recover" from it
		if errors.Is(err, vserr.ErrCollectionEmpty) {
			fmt.Printf("Dataset %q does not contain any documents\n", datasetIDs)
			return nil
		}
		return err
	}

	jsonSources, err := json.Marshal(retrievalResp.Responses)
	if err != nil {
		return err
	}

	fmt.Printf("Retrieved the following %d source collections for the original query %q: %s\n", len(retrievalResp.Responses), query, jsonSources)

	return nil
}
