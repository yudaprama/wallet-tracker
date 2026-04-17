package repository

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const etherscanAPIURL = "https://api.etherscan.io/v2/api"

type EtherscanTokenHolder struct {
	TokenHolderAddress     string `json:"TokenHolderAddress"`
	TokenHolderQuantity    string `json:"TokenHolderQuantity"`
	TokenHolderAddressType string `json:"TokenHolderAddressType"`
}

type EtherscanTokenInfo struct {
	ContractAddress string `json:"contractAddress"`
	TokenName       string `json:"tokenName"`
	Symbol          string `json:"symbol"`
	Divisor         string `json:"divisor"`
	TotalSupply     string `json:"totalSupply"`
}

type EtherscanTokenBalance struct {
	Address       string
	QuantityRaw   string
	Quantity      string
	TokenName     string
	TokenSymbol   string
	TokenDecimals int
}

type etherscanTopHoldersResponse struct {
	Status  string                 `json:"status"`
	Message string                 `json:"message"`
	Result  []EtherscanTokenHolder `json:"result"`
}

type etherscanErrorResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type etherscanTokenInfoResponse struct {
	Status  string               `json:"status"`
	Message string               `json:"message"`
	Result  []EtherscanTokenInfo `json:"result"`
}

type etherscanTokenBalanceResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"`
}

func FetchTopTokenHolders(apiKey, contractAddress string, limit int) ([]EtherscanTokenHolder, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("ETHERSCAN_API_KEY is required")
	}

	if CheckWalletNetwork(contractAddress) != EthNetwork {
		return nil, fmt.Errorf("contract address must be a valid Ethereum address")
	}

	if limit <= 0 {
		limit = 20
	}

	values := url.Values{}
	values.Set("chainid", "1")
	values.Set("module", "token")
	values.Set("action", "topholders")
	values.Set("contractaddress", contractAddress)
	values.Set("offset", strconv.Itoa(limit))
	values.Set("apikey", apiKey)

	req, err := http.NewRequest(http.MethodGet, etherscanAPIURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("etherscan returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var okResp etherscanTopHoldersResponse
	if err := json.Unmarshal(body, &okResp); err == nil && okResp.Status == "1" {
		return okResp.Result, nil
	}

	var errResp etherscanErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, fmt.Errorf("failed to decode etherscan response: %w", err)
	}

	resultText := strings.TrimSpace(string(errResp.Result))
	resultText = strings.Trim(resultText, `"`)

	if resultText == "" {
		resultText = "unknown error"
	}

	if strings.Contains(strings.ToLower(resultText), "api pro endpoint") {
		return nil, fmt.Errorf(
			"etherscan top holders requires a paid Pro plan; free API keys cannot access module=token&action=topholders. Use the Etherscan Holders page manually or upgrade the API plan",
		)
	}

	return nil, fmt.Errorf("etherscan error: %s (%s)", errResp.Message, resultText)
}

func FetchTokenInfo(apiKey, contractAddress string) (EtherscanTokenInfo, error) {
	values := url.Values{}
	values.Set("chainid", "1")
	values.Set("module", "token")
	values.Set("action", "tokeninfo")
	values.Set("contractaddress", contractAddress)
	values.Set("apikey", apiKey)

	body, err := fetchEtherscanBody(values)
	if err != nil {
		return EtherscanTokenInfo{}, err
	}

	var okResp etherscanTokenInfoResponse
	if err := json.Unmarshal(body, &okResp); err == nil && okResp.Status == "1" && len(okResp.Result) > 0 {
		return okResp.Result[0], nil
	}

	return EtherscanTokenInfo{}, decodeEtherscanError(body)
}

func FetchTokenBalance(apiKey, contractAddress, address string) (string, error) {
	values := url.Values{}
	values.Set("chainid", "1")
	values.Set("module", "account")
	values.Set("action", "tokenbalance")
	values.Set("contractaddress", contractAddress)
	values.Set("address", address)
	values.Set("tag", "latest")
	values.Set("apikey", apiKey)

	body, err := fetchEtherscanBody(values)
	if err != nil {
		return "", err
	}

	var okResp etherscanTokenBalanceResponse
	if err := json.Unmarshal(body, &okResp); err == nil && okResp.Status == "1" {
		return okResp.Result, nil
	}

	return "", decodeEtherscanError(body)
}

func fetchEtherscanBody(values url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, etherscanAPIURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("etherscan returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func decodeEtherscanError(body []byte) error {
	var errResp etherscanErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("failed to decode etherscan response: %w", err)
	}

	resultText := strings.TrimSpace(string(errResp.Result))
	resultText = strings.Trim(resultText, `"`)

	if resultText == "" {
		resultText = "unknown error"
	}

	if strings.Contains(strings.ToLower(resultText), "api pro endpoint") {
		return fmt.Errorf(
			"etherscan top holders requires a paid Pro plan; free API keys cannot access module=token&action=topholders. Use the Etherscan Holders page manually or upgrade the API plan",
		)
	}

	return fmt.Errorf("etherscan error: %s (%s)", errResp.Message, resultText)
}

func FormatTokenQuantity(raw string, decimals int) string {
	if decimals <= 0 {
		return raw
	}

	value := new(big.Int)
	if _, ok := value.SetString(raw, 10); !ok {
		return raw
	}

	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	rat := new(big.Rat).SetFrac(value, divisor)
	return rat.FloatString(decimals)
}

func CompareTokenBalancesDesc(left, right EtherscanTokenBalance) bool {
	leftInt := new(big.Int)
	rightInt := new(big.Int)

	if _, ok := leftInt.SetString(left.QuantityRaw, 10); !ok {
		return left.QuantityRaw > right.QuantityRaw
	}
	if _, ok := rightInt.SetString(right.QuantityRaw, 10); !ok {
		return left.QuantityRaw > right.QuantityRaw
	}

	return leftInt.Cmp(rightInt) > 0
}
