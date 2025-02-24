package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

const (
	iosURLTemplate = locoBaseURL + "/export/archive/%s.zip"
	plistFilename  = "InfoPlist.strings"
	locoFormatYML  = "yml"
)

var (
	// special things we need for the right locale names. these are overrides checked before the locales map.
	iosLocaleMap = map[string]string{
		"pt-BR":  "pt",
		"tw":     "ak",
		"zh-CN":  "zh-Hans",
		"tl":     "fil",
		"vec-BR": "vec",
	}

	// this maps the loco tag filter to the file type we're asking for
	filterTypeMap = map[string]string{
		"iOS-strings": "strings",
		"iOS-plurals": "stringsdict",
		"iOS-plist":   locoFormatYML,
	}

	plistAssetMap = map[string][]string{
		"touchid.authentication-prompt":              {"NSFaceIDUsageDescription"},
		"schedules.territory.current-location-usage": {"NSLocationWhenInUseUsageDescription"},
		"mobile.calendar.usage":                      {"NSCalendarsFullAccessUsageDescription", "NSCalendarsUsageDescription"},
		"mobile.camera.usage":                        {"NSCameraUsageDescription"},
		"shortcut.last-month":                        {"shortcut.last-month"},
		"Hourglass":                                  {bundleNameAsset},
	}
)

func updateiOSAssets(apiKey, baseDir string) error {
	// verify that baseDir is valid
	if !isValidDir(baseDir) {
		return fmt.Errorf("invalid base dir: %s", baseDir)
	}

	getTranslations := func(tag, format string) (*http.Response, error) {
		qp := url.Values{}
		qp.Set(locoFilter, tag)
		qp.Set("index", "id")
		qp.Set("fallback", locoFallback)

		locoURL := fmt.Sprintf(iosURLTemplate, format)
		resp, err := locoRequest(apiKey, locoURL, qp)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(filterTypeMap))
	successCount := atomic.Int32{}
	for k, v := range filterTypeMap {
		tag := k
		format := v
		go func() {
			defer wg.Done()
			resp, err := getTranslations(tag, format)
			slog.Info("fetch done", slog.Int("status", resp.StatusCode))
			if err != nil || resp.StatusCode != http.StatusOK {
				slog.Error("error getting", slog.String("tag", tag),
					slog.Int("status", resp.StatusCode), slog.Any("err", err.Error()))
			} else {
				slog.Info("processing", slog.String("tag", tag))
				err = processTranslations(format, baseDir, resp)
				if err != nil {
					slog.Error("error processing", slog.String("tag", tag), slog.Any("err", err.Error()))
				} else {
					successCount.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if int(successCount.Load()) != len(filterTypeMap) {
		return fmt.Errorf("did not process %d sets as expected", len(filterTypeMap))
	}

	return nil
}

func processTranslations(format, baseDir string, resp *http.Response) error {
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

	for _, zipFile := range zipReader.File {
		dir, zipName := filepath.Split(zipFile.Name)
		ext := strings.TrimPrefix(filepath.Ext(zipName), ".")
		if ext != format {
			slog.Debug("skipping", slog.String("file", zipFile.Name), slog.String("ext", ext))
			continue
		}

		locale := localefrompath_iOS(dir)
		if locale == "" {
			slog.Error("cannot find locale",
				slog.String("dir", dir))
			continue
		}

		outputDir := filepath.Join(baseDir, fmt.Sprintf("%s.lproj", locale))
		if !isValidDir(outputDir) {
			slog.Error("cannot find output directory",
				slog.String("dir", outputDir))
			continue
		}

		f, zipErr := zipFile.Open()
		if zipErr != nil {
			slog.Error("error opening file",
				slog.String("file", zipFile.Name), slog.Any("err", zipErr))
			continue
		}

		fileData, readErr := io.ReadAll(f)
		if readErr != nil {
			slog.Error("error reading zip data for file",
				slog.String("file", zipFile.Name), slog.Any("err", readErr))
			f.Close()
			continue
		}

		outputFilename := zipName
		if format == locoFormatYML {
			outputFilename = plistFilename
		}
		outFilePath := filepath.Join(outputDir, outputFilename)
		outFile, fileErr := os.OpenFile(outFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if fileErr != nil {
			slog.Error("error creating file",
				slog.String("file", outFilePath), slog.Any("err", fileErr))
		} else {
			slog.Debug("opened file", slog.String("file", outFilePath))
			if format == locoFormatYML {
				fileErr = processPlistYaml(fileData, outFile)
			} else {
				_, fileErr = outFile.Write(fileData)
			}
			if fileErr != nil {
				slog.Error("error writing to file",
					slog.String("file", outFilePath), slog.Any("err", fileErr))
			}
			outFile.Close()
		}
		f.Close()
	}

	return nil
}

func localefrompath_iOS(dir string) string {
	baseName := filepath.Base(dir)
	rawLocaleName := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	return iosLocale(rawLocaleName)
}

func iosLocale(rawLocaleName string) string {
	if overrideLocale, ok := iosLocaleMap[rawLocaleName]; ok {
		return overrideLocale
	}

	locale := locales[rawLocaleName]
	if locale != "" {
		return locale
	}
	return rawLocaleName
}

func processPlistYaml(fileData []byte, outFile *os.File) error {
	translations := make(map[string]string)
	err := yaml.Unmarshal(fileData, translations)
	if err != nil {
		return err
	}

	for asset, translation := range translations {
		plistName := plistAssetMap[asset]
		if len(plistName) == 0 {
			slog.Error("asset not found in plist map", slog.String("asset", asset))
			continue
		}
		for _, plKey := range plistName {
			_, err = outFile.WriteString(fmt.Sprintf(`"%s" = "%s";`+"\n", plKey, translation))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
