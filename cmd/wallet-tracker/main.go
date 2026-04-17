package main

import (
	"fmt"
	generic "github.com/aydinnyunus/wallet-tracker/cli/command/repository"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
	"log"
	"os"
	"strings"

	"github.com/aydinnyunus/wallet-tracker/cli/command/commands"
	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

func main() {
	_ = godotenv.Load()

	rootCmd = commands.NewWalletTrackerCommand()

	if requiresNeo4j(os.Args[1:]) {
		neo4jUser := generic.GetEnv("NEO4J_USERNAME", "neo4j")
		neo4jPass := generic.GetEnv("NEO4J_PASS", "letmein")

		if generic.ContainerExists(generic.ContainerName) {
			dockerEnvVarValue, err := generic.GetDockerEnvVar(generic.ContainerName, generic.Neo4jAuth)
			if err != nil {
				log.Fatalf("Error getting Docker env var: %v", err)
			}

			envVarValue := neo4jUser + "/" + neo4jPass

			if envVarValue == dockerEnvVarValue {
				fmt.Println("The .env NEO4J_AUTH value matches the Docker container NEO4J_AUTH value.")
			} else {
				generic.RestartDockerCompose()
			}
		} else {
			color.Red("Please start neo4j database using ./wallet-tracker neodash start")
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func requiresNeo4j(args []string) bool {
	if len(args) == 0 {
		return false
	}

	commandPath := strings.Join(args, " ")

	return strings.HasPrefix(commandPath, "neodash") ||
		strings.HasPrefix(commandPath, "tracker track") ||
		strings.HasPrefix(commandPath, "tracker websocket")
}
