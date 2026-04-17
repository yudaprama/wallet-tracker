package tracker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/spf13/cobra"
)

func TokenBalancesCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "token-balances",
		Short: "Check ERC-20 balances for a list of addresses",
		Long:  `Check ERC-20 balances for a list of addresses using the free Etherscan token balance endpoint.`,
		RunE:  startTokenBalances,
	}

	getCmd.Flags().String(
		"contract", "", "Specify ERC-20 contract address",
	)
	getCmd.Flags().StringSlice(
		"address", nil, "Repeatable holder address flag",
	)
	getCmd.Flags().String(
		"addresses", "", "Whitespace or comma separated holder addresses",
	)
	getCmd.Flags().String(
		"addresses-file", "", "Path to a file containing holder addresses separated by whitespace or newlines",
	)
	getCmd.Flags().Int(
		"top", 20, "Show only top N balances after sorting",
	)
	getCmd.Flags().Int(
		"decimals", 18, "Token decimals used to format balances when token metadata is unavailable",
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

func startTokenBalances(cmd *cobra.Command, _ []string) error {
	contractAddress, err := cmd.Flags().GetString("contract")
	if err != nil {
		return err
	}

	flagAddresses, err := cmd.Flags().GetStringSlice("address")
	if err != nil {
		return err
	}

	inlineAddresses, err := cmd.Flags().GetString("addresses")
	if err != nil {
		return err
	}

	addressesFile, err := cmd.Flags().GetString("addresses-file")
	if err != nil {
		return err
	}

	fileAddresses, err := loadAddressesFile(addressesFile)
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

	top, err := cmd.Flags().GetInt("top")
	if err != nil {
		return err
	}

	decimals, err := cmd.Flags().GetInt("decimals")
	if err != nil {
		return err
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	addresses := normalizeAddresses(flagAddresses, strings.TrimSpace(inlineAddresses+" "+fileAddresses))
	if len(addresses) == 0 {
		return fmt.Errorf("at least one holder address is required via --address, --addresses, or --addresses-file")
	}

	results, _, err := fetchBalancesForAddresses(apiKey, contractAddress, addresses, decimals)
	if err != nil {
		return err
	}

	if top > 0 && len(results) > top {
		results = results[:top]
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	case "table":
		printTokenBalancesTable(results)
		return nil
	default:
		return fmt.Errorf("unsupported format %q, use table or json", format)
	}
}

func printTokenBalancesTable(balances []repository.EtherscanTokenBalance) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "RANK\tADDRESS\tBALANCE\tRAW")
	for index, balance := range balances {
		_, _ = fmt.Fprintf(
			writer,
			"%d\t%s\t%s\t%s\n",
			index+1,
			balance.Address,
			balance.Quantity,
			balance.QuantityRaw,
		)
	}
	_ = writer.Flush()
}
