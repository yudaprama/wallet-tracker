package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/spf13/cobra"
)

func TokenHoldersCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "token-holders",
		Short: "Fetch top ERC-20 holders from Etherscan",
		Long:  `Fetch the top ERC-20 token holders using the Etherscan token holders API.`,
		RunE:  startTokenHolders,
	}

	getCmd.Flags().String(
		"contract", "", "Specify ERC-20 contract address",
	)
	getCmd.Flags().Int(
		"limit", 20, "Number of top holders to fetch",
	)
	getCmd.Flags().String(
		"api-key", "", "Specify Etherscan API key, or use ETHERSCAN_API_KEY from environment",
	)
	getCmd.Flags().String(
		"format", "table", "Output format: table or json",
	)

	_ = getCmd.MarkFlagRequired("contract")

	return getCmd
}

func startTokenHolders(cmd *cobra.Command, _ []string) error {
	contractAddress, err := cmd.Flags().GetString("contract")
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	apiKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return err
	}
	if apiKey == "" {
		apiKey = repository.GetEnv("ETHERSCAN_API_KEY", "")
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	holders, err := repository.FetchTopTokenHolders(apiKey, contractAddress, limit)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(holders)
	case "table":
		printTokenHoldersTable(holders)
		return nil
	default:
		return fmt.Errorf("unsupported format %q, use table or json", format)
	}
}

func printTokenHoldersTable(holders []repository.EtherscanTokenHolder) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "RANK\tADDRESS\tQUANTITY\tTYPE")
	for index, holder := range holders {
		_, _ = fmt.Fprintf(
			writer,
			"%d\t%s\t%s\t%s\n",
			index+1,
			holder.TokenHolderAddress,
			holder.TokenHolderQuantity,
			holder.TokenHolderAddressType,
		)
	}
	_ = writer.Flush()
}
