package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/config"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  `Commands for viewing and managing panel configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run:   runConfigShow,
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a configuration value",
	Run:   runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set a configuration value",
	Run:   runConfigSet,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	Run:   runConfigPath,
}

var (
	configKey   string
	configValue string
)

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configPathCmd)

	configGetCmd.Flags().StringVarP(&configKey, "key", "k", "", "Configuration key (required)")
	configGetCmd.MarkFlagRequired("key")

	configSetCmd.Flags().StringVarP(&configKey, "key", "k", "", "Configuration key (required)")
	configSetCmd.Flags().StringVarP(&configValue, "value", "v", "", "Configuration value (required)")
	configSetCmd.MarkFlagRequired("key")
	configSetCmd.MarkFlagRequired("value")
}

func loadConfig() *viper.Viper {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(config.EtcRoot())
	v.AddConfigPath(".")

	// Environment variables
	v.SetEnvPrefix("VKAI")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			printError("Failed to read config: %v", err)
		}
	}

	return v
}

func runConfigShow(cmd *cobra.Command, args []string) {
	v := loadConfig()

	fmt.Println("=== VKAI Panel Configuration ===")
	fmt.Println()
	fmt.Printf("Config File: %s\n", v.ConfigFileUsed())
	fmt.Println()

	settings := v.AllSettings()
	printMap(settings, "")
}

func printMap(m map[string]interface{}, prefix string) {
	for key, value := range m {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			fmt.Printf("\n[%s]\n", fullKey)
			printMap(v, fullKey)
		default:
			// Mask sensitive values
			if isSensitiveKey(key) {
				fmt.Printf("%s = ****\n", fullKey)
			} else {
				fmt.Printf("%s = %v\n", fullKey, value)
			}
		}
	}
}

func isSensitiveKey(key string) bool {
	sensitiveKeys := []string{"password", "secret", "token", "key", "dsn", "url"}
	lowerKey := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lowerKey, s) {
			return true
		}
	}
	return false
}

func runConfigGet(cmd *cobra.Command, args []string) {
	v := loadConfig()

	value := v.Get(configKey)
	if value == nil {
		printError("Configuration key not found: %s", configKey)
	}

	if isSensitiveKey(configKey) {
		fmt.Printf("%s = ****\n", configKey)
	} else {
		fmt.Printf("%s = %v\n", configKey, value)
	}
}

func runConfigSet(cmd *cobra.Command, args []string) {
	v := loadConfig()

	v.Set(configKey, configValue)

	configFile := v.ConfigFileUsed()
	if configFile == "" {
		configFile = config.ConfigFile()
	}

	if err := v.WriteConfigAs(configFile); err != nil {
		printError("Failed to write config: %v", err)
	}

	printSuccess("Configuration updated: %s = %s", configKey, configValue)
	printInfo("Restart the API service to apply changes: systemctl restart vkai-api")
}

func runConfigPath(cmd *cobra.Command, args []string) {
	v := loadConfig()
	fmt.Println(v.ConfigFileUsed())
}
