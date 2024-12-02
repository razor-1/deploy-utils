package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
		slog.Error("error reading all response bytes", slog.Any("err", err))
		return err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		slog.Error("zip.NewReader error", slog.Any("err", err))
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
			slog.Error("error opening file",
				slog.String("file", zipFile.Name), slog.Any("err", err))
			continue
		}
		yamlData[localeCode], err = io.ReadAll(f)
		if err != nil {
			slog.Error("error reading zip data for file",
				slog.String("file", zipFile.Name), slog.Any("err", err))
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
			slog.Error("error creating output file",
				slog.String("file", filename), slog.Any("err", err))
			continue
		}
		if outFile == nil {
			// this happens when we skip something - not an error, but e.g. a locale we don't want to process
			continue
		}

		yamlMap := make(map[string]string)
		err = yaml.Unmarshal(data, &yamlMap)
		if err != nil {
			slog.Error("error unmarshalling yaml",
				slog.String("file", filename), slog.Any("err", err))
			outFile.Close()
			continue
		}

		err = writeYamlFile(yamlMap, outFile)
		if err != nil {
			slog.Error("error writing output file",
				slog.String("file", filename), slog.Any("err", err))
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
