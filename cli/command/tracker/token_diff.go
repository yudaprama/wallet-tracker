package tracker

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/spf13/cobra"
)

func TokenDiffCommand() *cobra.Command {
	getCmd := &cobra.Command{
		Use:   "token-diff",
		Short: "Compare the latest two snapshot batches for a token",
		Long:  `Compare the latest two SQLite snapshot batches and show increases, decreases, and unchanged balances.`,
		RunE:  startTokenDiff,
	}

	getCmd.Flags().String("contract", "", "Specify ERC-20 contract address")
	getCmd.Flags().String("db", defaultTrackerDBPath, "SQLite database path for snapshots")
	getCmd.Flags().String("format", "table", "Output format: table or json")
	getCmd.Flags().String("direction", "changed", "Filter by direction: changed, increase, decrease, unchanged, all")
	getCmd.Flags().Int("top", 20, "Show only top N rows after sorting by absolute delta")

	_ = getCmd.MarkFlagRequired("contract")

	return getCmd
}

func startTokenDiff(cmd *cobra.Command, _ []string) error {
	contractAddress, err := cmd.Flags().GetString("contract")
	if err != nil {
		return err
	}

	dbPath, err := cmd.Flags().GetString("db")
	if err != nil {
		return err
	}

	format, err := cmd.Flags().GetString("format")
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

	previousBatch, currentBatch, err := repository.RequireTwoLatestSnapshotBatches(db, contractAddress)
	if err != nil {
		return err
	}

	previousSnapshots, err := repository.ListSnapshotsByBatch(db, contractAddress, previousBatch.BatchID)
	if err != nil {
		return err
	}
	currentSnapshots, err := repository.ListSnapshotsByBatch(db, contractAddress, currentBatch.BatchID)
	if err != nil {
		return err
	}

	allChanges := repository.BuildTokenBalanceChanges(previousBatch, currentBatch, previousSnapshots, currentSnapshots)
	changes := filterTokenBalanceChanges(allChanges, direction)

	if top > 0 && len(changes) > top {
		changes = changes[:top]
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(changes)
	case "table":
		fmt.Printf("previous batch: %s (%s)\n", previousBatch.BatchID, previousBatch.CapturedAt.Format(timeLayout))
		fmt.Printf("current batch:  %s (%s)\n", currentBatch.BatchID, currentBatch.CapturedAt.Format(timeLayout))
		printTokenDiffSummary(allChanges)
		printTokenDiffTable(changes)
		return nil
	default:
		return fmt.Errorf("unsupported format %q, use table or json", format)
	}
}

const timeLayout = "2006-01-02 15:04:05 MST"

const (
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	colorReset = "\033[0m"
)

func directionLabel(direction string) string {
	switch direction {
	case "increase":
		return colorGreen + "BUY" + colorReset
	case "decrease":
		return colorRed + "SELL" + colorReset
	default:
		return "—"
	}
}

func filterTokenBalanceChanges(changes []repository.TokenBalanceChange, direction string) []repository.TokenBalanceChange {
	if direction == "" || direction == "all" {
		return orderTokenBalanceChanges(excludeUnchangedChanges(changes))
	}

	filtered := make([]repository.TokenBalanceChange, 0, len(changes))
	for _, change := range changes {
		switch direction {
		case "changed":
			if change.Direction != "unchanged" {
				filtered = append(filtered, change)
			}
		case "increase", "decrease", "unchanged":
			if change.Direction == direction {
				filtered = append(filtered, change)
			}
		}
	}

	return orderTokenBalanceChanges(filtered)
}

func excludeUnchangedChanges(changes []repository.TokenBalanceChange) []repository.TokenBalanceChange {
	filtered := make([]repository.TokenBalanceChange, 0, len(changes))
	for _, change := range changes {
		if change.Direction != "unchanged" {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

func orderTokenBalanceChanges(changes []repository.TokenBalanceChange) []repository.TokenBalanceChange {
	sort.SliceStable(changes, func(i, j int) bool {
		leftPriority := directionPriority(changes[i].Direction)
		rightPriority := directionPriority(changes[j].Direction)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}

		leftAbs := new(big.Int).Abs(parseBigInt(changes[i].DeltaRaw))
		rightAbs := new(big.Int).Abs(parseBigInt(changes[j].DeltaRaw))
		return leftAbs.Cmp(rightAbs) > 0
	})
	return changes
}

func directionPriority(direction string) int {
	switch direction {
	case "increase":
		return 0
	case "decrease":
		return 1
	case "unchanged":
		return 2
	default:
		return 3
	}
}

func printTokenDiffSummary(changes []repository.TokenBalanceChange) {
	if len(changes) == 0 {
		fmt.Println("summary: no matching movements")
		return
	}

	decimals := changes[0].Decimals
	increaseCount := 0
	decreaseCount := 0
	unchangedCount := 0
	increaseTotal := big.NewInt(0)
	decreaseTotal := big.NewInt(0)
	netTotal := big.NewInt(0)

	for _, change := range changes {
		delta := parseBigInt(change.DeltaRaw)
		netTotal.Add(netTotal, delta)

		switch change.Direction {
		case "increase":
			increaseCount++
			increaseTotal.Add(increaseTotal, delta)
		case "decrease":
			decreaseCount++
			decreaseTotal.Add(decreaseTotal, new(big.Int).Abs(delta))
		default:
			unchangedCount++
		}
	}

	fmt.Printf(
		"summary: %s%s%s %d increase, %s%s%s %d decrease, %d unchanged | inflow %s | outflow -%s | net %s\n",
		colorGreen, "▴", colorReset, increaseCount,
		colorRed, "▾", colorReset, decreaseCount,
		unchangedCount,
		repository.FormatCompactSignedTokenQuantity(increaseTotal.String(), decimals),
		repository.FormatCompactTokenQuantity(decreaseTotal.String(), decimals),
		repository.FormatCompactSignedTokenQuantity(netTotal.String(), decimals),
	)
}

func printTokenDiffTable(changes []repository.TokenBalanceChange) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, "RANK\tADDRESS\tPREVIOUS\tCURRENT\tDELTA\tACTION")
	for index, change := range changes {
		deltaColor := colorReset
		if change.Direction == "increase" {
			deltaColor = colorGreen
		} else if change.Direction == "decrease" {
			deltaColor = colorRed
		}

		_, _ = fmt.Fprintf(
			writer,
			"%d\t%s\t%s\t%s\t%s%s%s\t%s\n",
			index+1,
			change.WalletAddress,
			change.PreviousBalance,
			change.CurrentBalance,
			deltaColor, change.Delta, colorReset,
			directionLabel(change.Direction),
		)
	}
	_ = writer.Flush()
}

func parseBigInt(value string) *big.Int {
	result := new(big.Int)
	if _, ok := result.SetString(value, 10); !ok {
		return big.NewInt(0)
	}
	return result
}
