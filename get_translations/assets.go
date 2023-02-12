package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	tplName       = "assets.tpl"
	locoAssetsURL = locoBaseURL + "/assets"
)

var (
	namedParameter = regexp.MustCompile(`%\((\w+?)\)(\S)?`)
	numberFirst    = regexp.MustCompile(`^\d`)
)

type LocoAsset struct {
	ID           string `json:"id"` // this is all we care about right now. actual loco json has much more info
	GoIdentifier string `json:"-"`
}

// pull down the assets from loco, and create a go file with all their names as constants
func generateAssets(apiKey string, args []string) error {
	locoAssets, err := getAssets(apiKey)
	if err != nil {
		return err
	}

	for i, asset := range locoAssets {
		locoAssets[i].GoIdentifier = validConstant(asset.ID)
	}

	tmpl, err := template.New("assets.tpl").ParseFiles(tplName)
	if err != nil {
		return err
	}

	outFile, err := os.Create(args[0])
	if err != nil {
		return err
	}
	defer outFile.Close()
	return tmpl.Execute(outFile, locoAssets)
}

func getAssets(apiKey string) (assets []LocoAsset, err error) {
	resp, err := locoRequest(apiKey, locoAssetsURL, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	assets = make([]LocoAsset, 0, 1000)
	jd := json.NewDecoder(resp.Body)
	err = jd.Decode(&assets)

	return
}

// create a valid Go identifier
func validConstant(assetID string) string {
	spaces := strings.Fields(assetID)
	titleCaser := cases.Title(language.English)
	var identifier string
	for _, field := range spaces {
		if strings.HasPrefix(field, "%") {
			matches := namedParameter.FindStringSubmatch(field)
			if len(matches) > 1 {
				identifier += "_" + titleCaser.String(matches[1])
			}
			continue
		}
		dots := strings.Split(field, ".")
		for _, dot := range dots {
			identifier += titleCaser.String(dot)
		}
	}

	if numberFirst.MatchString(identifier) {
		identifier = "_" + identifier
	}
	return strings.ReplaceAll(identifier, "-", "")
}
