package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

/*
This does one of these things:
1. Downloads the translations in PO gettext format from loco into the directory specified on the command line. Used
for pulling translations into the running container for deployment.
This is the "po" command mode.

2. Generates the locales/asset_ids.go file. Usually run via go generate.
This is the "assets" command mode.

3. Pulls down the i18nextv4 format from loco and writes each locale to a separate json file.
This is the "json" command mode.

4. Pulls down the yaml format for use with hugo.
This is the "hugoyaml" command mode.

5. Creates the list of BCP 47 fallback locales for each language.
This is the "fallback" command mode.

6. Pulls down the Android format and writes it into the resource directories.
This is the "android" command mode.

7. Pulls down the iOS strings and stringsdict and writes it into the Xcode project.
This is the "ios" command mode.

8. Pulls down the Xcode string catalogs and writes them.
This is the "ioscat" command mode.

9. Updates all the translations for an asset to change from python-style to i18next style formatting
This is the "i18conv" command mode. Note that it requires an API key that allows writing.
*/

const (
	// #nosec G101 // this is not a credential
	apiKeyVar   = "LOCO_RO_API_KEY"
	authHeader  = "Authorization"
	locoBaseURL = "https://localise.biz/api"
	tagMobile   = "mobile-apps"
)

func main() {
	apiKey := os.Getenv(apiKeyVar)
	if apiKey == "" {
		fmt.Printf("missing api key: provide it in the environment variable %s\n", apiKeyVar)
		os.Exit(1)
	}

	rootCmd := &cobra.Command{
		Use: "get_translations",
	}
	poCmd := &cobra.Command{
		Use: "po <directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getPOExport(apiKey, args)
		},
		Args: cobra.ExactArgs(1),
	}
	assetsCmd := &cobra.Command{
		Use: "assets <file.go>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateAssets(apiKey, args)
		},
		Args: cobra.ExactArgs(1),
	}
	jsonCmd := &cobra.Command{
		Use: "json <directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return getI18Next(apiKey, args[0], args[1])
			} else {
				return getI18Next(apiKey, args[0], "")
			}
		},
		Args: cobra.MinimumNArgs(1),
	}
	hugoYamlCmd := &cobra.Command{
		Use: "hugoyaml <directory> [tag]",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return getHugoYaml(apiKey, args[0], args[1])
			} else {
				return getHugoYaml(apiKey, args[0], "")
			}
		},
		Args: cobra.MinimumNArgs(1),
	}

	fallbackCmd := &cobra.Command{
		Use: "fallback",
		RunE: func(cmd *cobra.Command, args []string) error {
			return getFallbackLangs(apiKey)
		},
	}

	androidCmd := &cobra.Command{
		Use: "android <directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateAndroidAssets(apiKey, args[0], tagMobile)
		},
		Args: cobra.MinimumNArgs(1),
	}

	iosCmd := &cobra.Command{
		Use: "ios <directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateiOSAssets(apiKey, args[0])
		},
		Args: cobra.MinimumNArgs(1),
	}

	iosCatCmd := &cobra.Command{
		Use: "ioscat <directory>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return updateiOSAssetsCatalog(apiKey, args[0])
		},
		Args: cobra.MinimumNArgs(1),
	}

	i18ConvCmd := &cobra.Command{
		Use: "i18conv <asset>",
		RunE: func(cmd *cobra.Command, args []string) error {
			var formatKey string
			if len(args) > 1 {
				formatKey = args[1]
			}
			return i18nextConvertFormat(apiKey, args[0], formatKey)
		},
		Args: cobra.MinimumNArgs(1),
	}

	rootCmd.AddCommand(poCmd, assetsCmd, jsonCmd, hugoYamlCmd, fallbackCmd, androidCmd, iosCmd, iosCatCmd, i18ConvCmd)
	err := rootCmd.Execute()
	if err != nil {
		panic(err)
	}
}
