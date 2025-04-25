package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"golang.org/x/text/language"
)

const (
	locoJsonExportURL = locoBaseURL + "/export/all.json"
	locoI18NextFormat = "i18next4"
	locoProject       = "hourglass"
)

// retrieve loco assets in i18next format and write each locale's data to a separate json file
func getI18Next(apiKey, dir, filter string) error {
	qp := url.Values{}
	qp.Add("format", locoI18NextFormat)
	qp.Add("fallback", locoFallback)
	// printf causes the python and other formatting to be converted to i18next
	qp.Add("printf", "i18next")
	if filter != "" {
		qp.Add(locoFilter, filter)
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
			langFile := locales[locale]
			if langFile == "" {
				langFile = locale
				fmt.Printf("could not find locale mapping for %s. using %s\n", locale, langFile)
			}

			fileName := filepath.Join(dir, fmt.Sprintf("%s.json", langFile))
			err = writeToFile(fileName, data)
			if err != nil {
				return err
			}
			l, err := language.Parse(langFile)
			if err != nil {
				return fmt.Errorf("language.Parse failed for %s: %v", locale, err)
			}
			if l.String() != langFile && l.String() != "ca-valencia" {
				// the ca-valencia hack is because some filesystems don't like the same file name with different case
				// go's language parsing ends up with a different code than we expect. copy the file out so that we have both.
				slog.Info("mismatch for code", slog.String("langFile", langFile),
					slog.String("string", l.String()))
				fileName = filepath.Join(dir, fmt.Sprintf("%s.json", l.String()))
				err = writeToFile(fileName, data)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func writeToFile(path string, data interface{}) error {
	outFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outFile.Close()
	je := json.NewEncoder(outFile)
	err = je.Encode(data)
	if err != nil {
		return err
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
