package repository

import (
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

type TokenBalanceSnapshot struct {
	BatchID         string    `json:"batch_id"`
	ContractAddress string    `json:"contract_address"`
	WalletAddress   string    `json:"wallet_address"`
	BalanceRaw      string    `json:"balance_raw"`
	Decimals        int       `json:"decimals"`
	CapturedAt      time.Time `json:"captured_at"`
}

type SnapshotBatch struct {
	BatchID    string    `json:"batch_id"`
	CapturedAt time.Time `json:"captured_at"`
}

type TokenBalanceChange struct {
	WalletAddress      string    `json:"wallet_address"`
	PreviousBalanceRaw string    `json:"previous_balance_raw"`
	CurrentBalanceRaw  string    `json:"current_balance_raw"`
	DeltaRaw           string    `json:"delta_raw"`
	Decimals           int       `json:"decimals"`
	PreviousBalance    string    `json:"previous_balance"`
	CurrentBalance     string    `json:"current_balance"`
	Delta              string    `json:"delta"`
	Direction          string    `json:"direction"`
	PreviousCapturedAt time.Time `json:"previous_captured_at"`
	CurrentCapturedAt  time.Time `json:"current_captured_at"`
}

func OpenTokenTrackerDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := EnsureTokenTrackerSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func EnsureTokenTrackerSchema(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS token_balance_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id TEXT NOT NULL,
    contract_address TEXT NOT NULL,
    wallet_address TEXT NOT NULL,
    balance_raw TEXT NOT NULL,
    decimals INTEGER NOT NULL,
    captured_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_balance_snapshots_contract_batch
ON token_balance_snapshots(contract_address, batch_id, captured_at);

CREATE INDEX IF NOT EXISTS idx_token_balance_snapshots_contract_wallet
ON token_balance_snapshots(contract_address, wallet_address, captured_at);
`

	_, err := db.Exec(schema)
	return err
}

func InsertTokenBalanceSnapshotBatch(db *sql.DB, batchID, contractAddress string, decimals int, capturedAt time.Time, balances []EtherscanTokenBalance) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO token_balance_snapshots (
    batch_id, contract_address, wallet_address, balance_raw, decimals, captured_at
) VALUES (?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	capturedAtText := capturedAt.UTC().Format(time.RFC3339Nano)
	for _, balance := range balances {
		if _, err := stmt.Exec(
			batchID,
			contractAddress,
			balance.Address,
			balance.QuantityRaw,
			decimals,
			capturedAtText,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func ListLatestSnapshotBatches(db *sql.DB, contractAddress string, limit int) ([]SnapshotBatch, error) {
	rows, err := db.Query(`
SELECT batch_id, captured_at
FROM token_balance_snapshots
WHERE contract_address = ?
GROUP BY batch_id, captured_at
ORDER BY captured_at DESC
LIMIT ?
`, contractAddress, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batches := make([]SnapshotBatch, 0)
	for rows.Next() {
		var batchID string
		var capturedAtText string
		if err := rows.Scan(&batchID, &capturedAtText); err != nil {
			return nil, err
		}

		capturedAt, err := time.Parse(time.RFC3339Nano, capturedAtText)
		if err != nil {
			return nil, err
		}

		batches = append(batches, SnapshotBatch{
			BatchID:    batchID,
			CapturedAt: capturedAt,
		})
	}

	return batches, rows.Err()
}

func ListSnapshotsByBatch(db *sql.DB, contractAddress, batchID string) (map[string]TokenBalanceSnapshot, error) {
	rows, err := db.Query(`
SELECT batch_id, contract_address, wallet_address, balance_raw, decimals, captured_at
FROM token_balance_snapshots
WHERE contract_address = ? AND batch_id = ?
`, contractAddress, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := make(map[string]TokenBalanceSnapshot)
	for rows.Next() {
		var snapshot TokenBalanceSnapshot
		var capturedAtText string
		if err := rows.Scan(
			&snapshot.BatchID,
			&snapshot.ContractAddress,
			&snapshot.WalletAddress,
			&snapshot.BalanceRaw,
			&snapshot.Decimals,
			&capturedAtText,
		); err != nil {
			return nil, err
		}

		capturedAt, err := time.Parse(time.RFC3339Nano, capturedAtText)
		if err != nil {
			return nil, err
		}
		snapshot.CapturedAt = capturedAt
		snapshots[snapshot.WalletAddress] = snapshot
	}

	return snapshots, rows.Err()
}

func BuildTokenBalanceChanges(previousBatch, currentBatch SnapshotBatch, previous, current map[string]TokenBalanceSnapshot) []TokenBalanceChange {
	addressSet := map[string]bool{}
	for address := range previous {
		addressSet[address] = true
	}
	for address := range current {
		addressSet[address] = true
	}

	addresses := make([]string, 0, len(addressSet))
	for address := range addressSet {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)

	changes := make([]TokenBalanceChange, 0, len(addresses))
	for _, address := range addresses {
		prev := previous[address]
		curr := current[address]

		decimals := curr.Decimals
		if decimals == 0 {
			decimals = prev.Decimals
		}

		prevRaw := prev.BalanceRaw
		currRaw := curr.BalanceRaw
		if prevRaw == "" {
			prevRaw = "0"
		}
		if currRaw == "" {
			currRaw = "0"
		}

		deltaRaw := subtractBigIntStrings(currRaw, prevRaw)
		changes = append(changes, TokenBalanceChange{
			WalletAddress:      address,
			PreviousBalanceRaw: prevRaw,
			CurrentBalanceRaw:  currRaw,
			DeltaRaw:           deltaRaw,
			Decimals:           decimals,
			PreviousBalance:    FormatCompactTokenQuantity(prevRaw, decimals),
			CurrentBalance:     FormatCompactTokenQuantity(currRaw, decimals),
			Delta:              FormatCompactSignedTokenQuantity(deltaRaw, decimals),
			Direction:          balanceDirection(deltaRaw),
			PreviousCapturedAt: previousBatch.CapturedAt,
			CurrentCapturedAt:  currentBatch.CapturedAt,
		})
	}

	sort.Slice(changes, func(i, j int) bool {
		return compareAbsoluteBigIntStrings(changes[i].DeltaRaw, changes[j].DeltaRaw) > 0
	})

	return changes
}

func subtractBigIntStrings(left, right string) string {
	leftInt := parseBigInt(left)
	rightInt := parseBigInt(right)
	return new(big.Int).Sub(leftInt, rightInt).String()
}

func compareAbsoluteBigIntStrings(left, right string) int {
	leftInt := new(big.Int).Abs(parseBigInt(left))
	rightInt := new(big.Int).Abs(parseBigInt(right))
	return leftInt.Cmp(rightInt)
}

func FormatSignedTokenQuantity(raw string, decimals int) string {
	value := parseBigInt(raw)
	sign := ""
	if value.Sign() > 0 {
		sign = "+"
	}
	return sign + FormatTokenQuantity(value.String(), decimals)
}

func FormatCompactTokenQuantity(raw string, decimals int) string {
	value := parseBigInt(raw)
	if decimals > 0 {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		value = new(big.Int).Quo(value, divisor)
	}
	return formatBigIntThousands(value.String())
}

func FormatCompactSignedTokenQuantity(raw string, decimals int) string {
	value := parseBigInt(raw)
	sign := ""
	if value.Sign() > 0 {
		sign = "+"
	}
	if value.Sign() < 0 {
		value = value.Abs(value)
		sign = "-"
	}
	if decimals > 0 {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
		value = new(big.Int).Quo(value, divisor)
	}
	return sign + formatBigIntThousands(value.String())
}

func balanceDirection(deltaRaw string) string {
	value := parseBigInt(deltaRaw)
	switch value.Sign() {
	case 1:
		return "increase"
	case -1:
		return "decrease"
	default:
		return "unchanged"
	}
}

func parseBigInt(value string) *big.Int {
	result := new(big.Int)
	if _, ok := result.SetString(value, 10); !ok {
		return big.NewInt(0)
	}
	return result
}

func formatBigIntThousands(value string) string {
	if value == "" {
		return "0"
	}

	sign := ""
	if value[0] == '-' {
		sign = "-"
		value = value[1:]
	}

	if len(value) <= 3 {
		return sign + value
	}

	output := make([]byte, 0, len(value)+len(value)/3)
	remainder := len(value) % 3
	if remainder > 0 {
		output = append(output, value[:remainder]...)
		if len(value) > remainder {
			output = append(output, ',')
		}
	}

	for i := remainder; i < len(value); i += 3 {
		output = append(output, value[i:i+3]...)
		if i+3 < len(value) {
			output = append(output, ',')
		}
	}

	return sign + string(output)
}

func RequireTwoLatestSnapshotBatches(db *sql.DB, contractAddress string) (SnapshotBatch, SnapshotBatch, error) {
	batches, err := ListLatestSnapshotBatches(db, contractAddress, 2)
	if err != nil {
		return SnapshotBatch{}, SnapshotBatch{}, err
	}
	if len(batches) < 2 {
		return SnapshotBatch{}, SnapshotBatch{}, fmt.Errorf("need at least 2 snapshots for contract %s before diff can be computed", contractAddress)
	}

	return batches[1], batches[0], nil
}
