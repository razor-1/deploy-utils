package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v2"
)

const (
	locoYamlExportURL = locoBaseURL + "/export/archive/yml.zip"
	locoYamlFormat    = "simple"
)

func getHugoYaml(apiKey, baseDir, filter string) error {
	qp := url.Values{}
	qp.Add("format", locoYamlFormat)
	qp.Add("fallback", locoFallback)
	qp.Add("index", "id")
	if filter != "" {
		qp.Add("filter", filter)
	}
	resp, err := locoRequest(apiKey, locoYamlExportURL, qp)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("error reading all response bytes: %v", err)
		return err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Errorf("zip.NewReader error: %v", err)
		return nil
	}

	yamlData := make(map[string][]byte)
	for _, zipFile := range zipReader.File {
		_, zipName := filepath.Split(zipFile.Name)
		ext := filepath.Ext(zipName)
		if ext != ".yml" {
			continue
		}

		localeCode := strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(zipName, ext), locoProject+"-"))

		f, err := zipFile.Open()
		if err != nil {
			log.Errorf("error opening file %s: %v", zipFile.Name, err)
			continue
		}
		yamlData[localeCode], err = io.ReadAll(f)
		if err != nil {
			log.Errorf("error reading zip data for file %s: %v", zipFile.Name, err)
		}
		f.Close()
	}

	for localeCode, data := range yamlData {
		filename := localeCode
		lang := language.MustParse(localeCode)
		baseLang, _ := lang.Base()
		if localesWithBase(yamlData, baseLang) == 1 {
			filename = baseLang.String()
		}

		outFile, err := os.Create(fmt.Sprintf("%s.yaml", filepath.Join(baseDir, filename)))
		if err != nil {
			log.Errorf("error creating output file for %s: %v", filename, err)
			continue
		}
		if outFile == nil {
			// this happens when we skip something - not an error, but e.g. a locale we don't want to process
			continue
		}

		yamlMap := make(map[string]string)
		err = yaml.Unmarshal(data, &yamlMap)
		if err != nil {
			log.Errorf("error unmarshalling yaml for %s: %v", filename, err)
			outFile.Close()
			continue
		}

		err = writeYamlFile(yamlMap, outFile)
		if err != nil {
			log.Errorf("error writing output file for %s: %v", filename, err)
		}
		outFile.Close()
	}
	return nil
}

type go18nFormat struct {
	Other string `yaml:"other"`
}

func writeYamlFile(yamlMap map[string]string, out io.Writer) error {
	// we need to create a new yaml structure where "other" is inserted, because go-i18n doesn't want to use any of the
	// standard formats.
	// right now we aren't supporting plurals since we don't have a need for them in this area.
	output := make(map[string]go18nFormat, len(yamlMap))
	for asset, translation := range yamlMap {
		output[asset] = go18nFormat{Other: translation}
	}

	ye := yaml.NewEncoder(out)
	return ye.Encode(output)
}
