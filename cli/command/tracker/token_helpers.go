package tracker

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aydinnyunus/wallet-tracker/cli/command/repository"
)

func normalizeAddresses(flagAddresses []string, inlineAddresses string) []string {
	seen := map[string]bool{}
	addresses := make([]string, 0)

	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			return
		}
		seen[strings.ToLower(value)] = true
		addresses = append(addresses, value)
	}

	for _, address := range flagAddresses {
		add(address)
	}

	replacer := strings.NewReplacer(",", " ", "\n", " ", "\t", " ")
	for _, address := range strings.Fields(replacer.Replace(inlineAddresses)) {
		add(address)
	}

	return addresses
}

func loadAddressesFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func fetchTokenBalanceWithRetry(apiKey, contractAddress, address string) (string, error) {
	var lastErr error

	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(1200 * time.Millisecond)
		}

		rawBalance, err := repository.FetchTokenBalance(apiKey, contractAddress, address)
		if err == nil {
			time.Sleep(350 * time.Millisecond)
			return rawBalance, nil
		}

		lastErr = err
		if !strings.Contains(strings.ToLower(err.Error()), "rate limit") {
			return "", err
		}
	}

	return "", lastErr
}

func fetchBalancesForAddresses(apiKey, contractAddress string, addresses []string, decimals int) ([]repository.EtherscanTokenBalance, int, error) {
	tokenInfo, err := repository.FetchTokenInfo(apiKey, contractAddress)
	if err == nil && tokenInfo.Divisor != "" {
		fmt.Sscanf(tokenInfo.Divisor, "%d", &decimals)
	}

	results := make([]repository.EtherscanTokenBalance, 0, len(addresses))
	for _, address := range addresses {
		rawBalance, balanceErr := fetchTokenBalanceWithRetry(apiKey, contractAddress, address)
		if balanceErr != nil {
			return nil, decimals, fmt.Errorf("failed to fetch %s: %w", address, balanceErr)
		}

		results = append(results, repository.EtherscanTokenBalance{
			Address:       address,
			QuantityRaw:   rawBalance,
			Quantity:      repository.FormatTokenQuantity(rawBalance, decimals),
			TokenName:     tokenInfo.TokenName,
			TokenSymbol:   tokenInfo.Symbol,
			TokenDecimals: decimals,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return repository.CompareTokenBalancesDesc(results[i], results[j])
	})

	return results, decimals, nil
}
