package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

/*
This does one of these things:
1. Downloads the translations in PO gettext format from loco into the directory specified on the command line. Used
for pulling translations into the running container for deployment.
This is the "po" command mode.

2. Generates the locales/asset_ids.go file. Usually run via go generate.
This is the "assets" command mode.

3. Pulls down the i18nextv3 format from loco and writes each locale to a separate json file.
This is the "json" command mode.

4. Pulls down the yaml format for use with hugo.
This is the "hugoyaml" command mode.
*/

const (
	// #nosec G101 // this is not a credential
	apiKeyVar   = "LOCO_RO_API_KEY"
	authHeader  = "Authorization"
	locoBaseURL = "https://localise.biz/api"
)

func main() {
	apiKey := os.Getenv(apiKeyVar)
	if apiKey == "" {
		log.Fatalf("missing api key: provide it in the environment variable %s", apiKeyVar)
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

	rootCmd.AddCommand(poCmd, assetsCmd, jsonCmd, hugoYamlCmd)
	err := rootCmd.Execute()
	if err != nil {
		log.Fatalf("error encountered: %v", err)
	}
}

func locoRequest(apiKey, URL string, queryParams url.Values) (resp *http.Response, err error) {
	reqURL, _ := url.Parse(URL)
	reqURL.RawQuery = queryParams.Encode()

	client := http.DefaultClient
	client.Timeout = 20 * time.Second
	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return
	}
	req.Header.Add(authHeader, fmt.Sprintf("Loco %s", apiKey))
	resp, err = client.Do(req)
	if err != nil {
		log.Fatalf("error fetching: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("status not OK: is %d", resp.StatusCode)
	}
	return
}
