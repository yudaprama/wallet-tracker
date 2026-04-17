package tracker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/spf13/cobra"
)

func TokenWatchCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "token-watch",
		Short: "Continuously track token balance movements",
		Long:  `Continuously fetch token balances on an interval, store snapshots in SQLite, and print balance movements after each cycle.`,
		RunE:  startTokenWatch,
	}

	getCmd.Flags().String("contract", "", "Specify ERC-20 contract address")
	getCmd.Flags().StringSlice("address", nil, "Repeatable holder address flag")
	getCmd.Flags().String("addresses", "", "Whitespace or comma separated holder addresses")
	getCmd.Flags().String("addresses-file", "data/rave-addresses.txt", "Path to a file containing holder addresses separated by whitespace or newlines")
	getCmd.Flags().Int("decimals", 18, "Token decimals used to format balances when token metadata is unavailable")
	getCmd.Flags().String("api-key", "", "Specify Etherscan API key, or use ETHERSCAN_API_KEY from environment")
	getCmd.Flags().String("db", defaultTrackerDBPath, "SQLite database path for snapshots")
	getCmd.Flags().Duration("interval", 5*time.Minute, "Polling interval between snapshots, for example 30s, 5m, 1h")
	getCmd.Flags().Int("iterations", 0, "Number of polling cycles to run. Use 0 to run until stopped")
	getCmd.Flags().String("direction", "changed", "Filter printed movements: changed, increase, decrease, unchanged, all")
	getCmd.Flags().Int("top", 20, "Show only top N rows after sorting by absolute delta")

	_ = getCmd.MarkFlagRequired("contract")

	return getCmd
}

func startTokenWatch(cmd *cobra.Command, _ []string) error {
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

	addresses := normalizeAddresses(flagAddresses, strings.TrimSpace(inlineAddresses+" "+fileAddresses))
	if len(addresses) == 0 {
		return fmt.Errorf("at least one holder address is required via --address, --addresses, or --addresses-file")
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

	interval, err := cmd.Flags().GetDuration("interval")
	if err != nil {
		return err
	}
	if interval <= 0 {
		return fmt.Errorf("interval must be greater than zero")
	}

	iterations, err := cmd.Flags().GetInt("iterations")
	if err != nil {
		return err
	}

	direction, err := cmd.Flags().GetString("direction")
	if err != nil {
		return err
	}

	top, err := cmd.Flags().GetInt("top")
	if err != nil {
		return err
	}

	db, err := repository.OpenTokenTrackerDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("watching %d addresses for contract %s every %s\n", len(addresses), contractAddress, interval)
	fmt.Printf("database: %s\n", dbPath)

	cycle := 0
	for {
		cycle++
		if err := runWatchCycle(db, apiKey, contractAddress, addresses, decimals, direction, top); err != nil {
			return err
		}

		if iterations > 0 && cycle >= iterations {
			return nil
		}

		select {
		case <-ctx.Done():
			fmt.Println("stopped token watch")
			return nil
		case <-time.After(interval):
		}
	}
}

func runWatchCycle(db *sql.DB, apiKey, contractAddress string, addresses []string, decimals int, direction string, top int) error {
	balances, decimals, err := fetchBalancesForAddresses(apiKey, contractAddress, addresses, decimals)
	if err != nil {
		return err
	}

	capturedAt := time.Now().UTC()
	batchID := capturedAt.Format("20060102T150405.000000000Z07:00")
	if err := repository.InsertTokenBalanceSnapshotBatch(db, batchID, contractAddress, decimals, capturedAt, balances); err != nil {
		return err
	}

	fmt.Printf("\ncycle snapshot: %s\n", batchID)

	previousBatch, currentBatch, err := repository.RequireTwoLatestSnapshotBatches(db, contractAddress)
	if err != nil {
		fmt.Println("first snapshot stored, waiting for next cycle to compute movement")
		return nil
	}

	previousSnapshots, err := repository.ListSnapshotsByBatch(db, contractAddress, previousBatch.BatchID)
	if err != nil {
		return err
	}
	currentSnapshots, err := repository.ListSnapshotsByBatch(db, contractAddress, currentBatch.BatchID)
	if err != nil {
		return err
	}

	changes := repository.BuildTokenBalanceChanges(previousBatch, currentBatch, previousSnapshots, currentSnapshots)
	changes = filterTokenBalanceChanges(changes, direction)
	if top > 0 && len(changes) > top {
		changes = changes[:top]
	}

	fmt.Printf("previous batch: %s (%s)\n", previousBatch.BatchID, previousBatch.CapturedAt.Format(timeLayout))
	fmt.Printf("current batch:  %s (%s)\n", currentBatch.BatchID, currentBatch.CapturedAt.Format(timeLayout))
	printTokenDiffTable(changes)

	return nil
}
