package tracker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aquasecurity/table"
	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/spf13/cobra"
)

const defaultTrackerDBPath = "data/token-tracker.sqlite"

func TokenSnapshotCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "token-snapshot",
		Short: "Fetch balances and store a snapshot batch in SQLite",
		Long:  `Fetch ERC-20 balances for a list of addresses and store the snapshot in SQLite for later diffing.`,
		RunE:  startTokenSnapshot,
	}

	getCmd.Flags().String("contract", "", "Specify ERC-20 contract address")
	getCmd.Flags().StringSlice("address", nil, "Repeatable holder address flag")
	getCmd.Flags().String("addresses", "", "Whitespace or comma separated holder addresses")
	getCmd.Flags().String("addresses-file", "", "Path to a file containing holder addresses separated by whitespace or newlines")
	getCmd.Flags().Int("decimals", 18, "Token decimals used to format balances when token metadata is unavailable")
	getCmd.Flags().String("api-key", "", "Specify Etherscan API key, or use ETHERSCAN_API_KEY from environment")
	getCmd.Flags().String("db", defaultTrackerDBPath, "SQLite database path for snapshots")
	getCmd.Flags().Int("top", 10, "Preview top N balances after snapshot is stored")

	_ = getCmd.MarkFlagRequired("contract")

	return getCmd
}

func startTokenSnapshot(cmd *cobra.Command, _ []string) error {
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

	decimals, err := cmd.Flags().GetInt("decimals")
	if err != nil {
		return err
	}

	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}

	top, err := cmd.Flags().GetInt("top")
	if err != nil {
		return err
	}

	addresses := normalizeAddresses(flagAddresses, strings.TrimSpace(inlineAddresses+" "+fileAddresses))
	if len(addresses) == 0 {
		return fmt.Errorf("at least one holder address is required via --address, --addresses, or --addresses-file")
	}

	balances, decimals, err := fetchBalancesForAddresses(apiKey, contractAddress, addresses, decimals)
	if err != nil {
		return err
	}

	db, err := repository.OpenTokenTrackerDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	capturedAt := time.Now().UTC()
	batchID := capturedAt.Format("20060102T150405.000000000Z07:00")
	if err := repository.InsertTokenBalanceSnapshotBatch(db, batchID, contractAddress, decimals, capturedAt, balances); err != nil {
		return err
	}

	fmt.Printf("saved snapshot batch %s to %s for %d addresses\n", batchID, dbPath, len(balances))
	printTokenBalancesPreview(balances, top)
	return nil
}

func printTokenBalancesPreview(balances []repository.EtherscanTokenBalance, top int) {
	if top <= 0 || len(balances) == 0 {
		return
	}
	if len(balances) > top {
		balances = balances[:top]
	}

	t := table.New(os.Stdout)
	t.SetHeaders("RANK", "ADDRESS", "BALANCE")

	for index, balance := range balances {
		t.AddRow(fmt.Sprintf("%d", index+1), balance.Address, balance.Quantity)
	}

	t.Render()
}
