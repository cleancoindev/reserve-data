package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/KyberNetwork/reserve-data"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:     "./cmd",
	Short:   "entry point to the application, required KYBER_ENV (default to dev) and KYBER_EXCHANGES as environment variables. if KYBER_EXCHANGE is not set, the core will be run without centralize exchanges",
	Example: "KYBER_ENV=dev KYBER_EXCHANGES=binance ./cmd command [flags]",
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	RootCmd.Flags().BoolP("verbose", "v", false, "verbose mode enable")

	var startServer = &cobra.Command{
		Use:   "server ",
		Short: "initiate the server with specific config",
		Long: `Start reserve-data core server with preset Environment and
Allow overwriting some parameter`,
		Example: "KYBER_ENV=dev KYBER_EXCHANGES=binance ./cmd server --noauth -p 8000",
		Run:     serverStart,
	}

	// start server flags.
	startServer.Flags().BoolVarP(&noAuthEnable, "noauth", "", false, "disable authentication")
	startServer.Flags().IntVarP(&servPort, "port", "p", 8000, "server port")
	startServer.Flags().StringVar(&endpointOW, "endpoint", "", "endpoint, default to configuration file")
	startServer.PersistentFlags().StringVar(&baseURL, "base_url", defaultBaseURL, "base_url for authenticated enpoint")
	startServer.Flags().BoolVarP(&stdoutLog, "log-to-stdout", "", false, "send log to both log file and stdout terminal")
	startServer.Flags().BoolVarP(&dryRun, "dryrun", "", false, "only test if all the configs are set correctly, will not actually run core")
	RootCmd.AddCommand(startServer)

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number of the application",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(reserve.VERSION)
		},
	}
	RootCmd.AddCommand(versionCmd)

	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}