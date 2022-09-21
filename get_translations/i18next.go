package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"golang.org/x/text/language"
)

const (
	locoJsonExportURL = locoBaseURL + "/export/all.json"
	locoI18NextFormat = "i18next4"
	locoProject       = "hourglass"
	locoFallback      = "en-US"
)

// retrieve loco assets in i18next format and write each locale's data to a separate json file
func getI18Next(apiKey, dir, filter string) error {
	qp := url.Values{}
	qp.Add("format", locoI18NextFormat)
	qp.Add("fallback", locoFallback)
	if filter != "" {
		qp.Add("filter", filter)
	}
	resp, err := locoRequest(apiKey, locoJsonExportURL, qp)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	localeCodes := make(map[string]map[string]interface{})
	jd := json.NewDecoder(resp.Body)
	err = jd.Decode(&localeCodes)
	if err != nil {
		return err
	}

	for locale, projects := range localeCodes {
		for project, data := range projects {
			if project != locoProject {
				return fmt.Errorf("got unexpected project in i18next response from loco: %s", project)
			}
			// what we want to do is write "en.json" if en-US is the only locale using en
			// but if we also have en-GB then the filenames will be en-US.json and en-GB.json
			// currently, pt-PT and pt-BR are the only locales in our system with the same base language
			lang := language.MustParse(locale)
			baseLang, _ := lang.Base()
			langFile := locale
			if localesWithBase(localeCodes, baseLang) == 1 {
				langFile = baseLang.String()
			}
			outFile, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s.json", langFile)))
			if err != nil {
				return err
			}
			je := json.NewEncoder(outFile)
			err = je.Encode(data)
			if err != nil {
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// count how many keys in localeCodes have the supplied base
func localesWithBase[V any](localeCodes map[string]V, base language.Base) int {
	count := 0
	for locale := range localeCodes {
		baseLang, _ := language.MustParse(locale).Base()
		if baseLang == base {
			count++
		}
	}

	return count
}
