package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	androidURL        = locoBaseURL + "/export/archive/xml.zip"
	locoAndroidFormat = "android"
)

var (
	// special things we need to do to get the proper output directory names
	androidLocaleMap = map[string]string{
		"pl-PL":    "pl",
		"sv-SE":    "sv",
		"da-DK":    "da",
		"lt-LT":    "lt",
		"ko-KR":    "ko",
		"cs-CZ":    "cs",
		"hr-HR":    "hr",
		"bg-BG":    "bg",
		"ja-JP":    "ja",
		"ro-RO":    "ro",
		"zh-CN":    "zh",
		"uk-UA":    "uk",
		"hu-HU":    "hu",
		"el-GR":    "el",
		"vi-VN":    "vi",
		"th-TH":    "th",
		"fi-FI":    "fi",
		"gu-IN":    "gu",
		"id-ID":    "in", // java is really cool and uses "in" for indonesian
		"tr-TR":    "tr",
		"zh-Hant":  "b+zh+Hant", // this is the android BCP 47 thing
		"rmn-Cyrl": "b+rmn+Cyrl",
		"he":       "iw", // another cool legacy java thing
	}

	androidResourceRegex = regexp.MustCompile("values-([a-z]{2,})-?r?([A-Za-z]{2,})?")
)

func updateAndroidAssets(apiKey, baseDir, tag string) error {
	// verify that baseDir is valid
	if !isValidDir(baseDir) {
		return fmt.Errorf("invalid base dir: %s", baseDir)
	}

	qp := url.Values{}
	qp.Add("format", locoAndroidFormat)
	qp.Add("fallback", locoFallback)
	qp.Add("index", "id")
	if tag != "" {
		qp.Add(locoFilter, tag)
	}
	resp, err := locoRequest(apiKey, androidURL, qp)
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

	for _, zipFile := range zipReader.File {
		dir, zipName := filepath.Split(zipFile.Name)
		ext := filepath.Ext(zipName)
		if ext != ".xml" {
			continue
		}

		slog.Info("dir", slog.String("dir", dir))

		outputDir := filepath.Join(baseDir, filepath.Base(dir))
		if !isValidDir(outputDir) {
			// output directory doesn't exist. we might need to map it
			matches := androidResourceRegex.FindStringSubmatch(filepath.Base(dir))
			matches = slices.DeleteFunc(matches, func(s string) bool { return s == "" })
			if len(matches) < 2 {
				slog.Error("cannot find matching resource for dir", slog.String("filename", zipFile.Name))
				continue
			}
			var locale, newOutputPath string
			if len(matches) == 3 {
				locale = fmt.Sprintf("%s-%s", matches[1], matches[2])
				newOutputPath = fmt.Sprintf("values-%s-r%s", matches[1], matches[2])
			} else if len(matches) == 2 {
				locale = matches[1]
				newOutputPath = fmt.Sprintf("values-%s", matches[1])
			}

			if mappedLocale, ok := androidLocaleMap[locale]; ok {
				newOutputPath = fmt.Sprintf("values-%s", mappedLocale)
			}

			outputDir = filepath.Join(baseDir, newOutputPath)
			if !isValidDir(outputDir) {
				slog.Error("cannot find matching resource for dir after mapping",
					slog.String("filename", zipFile.Name),
					slog.String("outputDir", outputDir),
					slog.String("locale", locale))
				continue
			}
		}

		f, zipErr := zipFile.Open()
		if zipErr != nil {
			slog.Error("error opening file",
				slog.String("file", zipFile.Name), slog.Any("err", zipErr))
			continue
		}

		xmlData, xmlErr := io.ReadAll(f)
		if xmlErr != nil {
			slog.Error("error reading zip data for file",
				slog.String("file", zipFile.Name), slog.Any("err", xmlErr))
			f.Close()
			continue
		}

		checkAndroidBlankTranslations(filepath.Base(dir), xmlData)

		outFilePath := filepath.Join(outputDir, "strings.xml")
		outFile, fileErr := os.OpenFile(outFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if fileErr != nil {
			slog.Error("error creating file",
				slog.String("file", outFilePath), slog.Any("err", fileErr))
		} else {
			_, fileErr = outFile.Write(xmlData)
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

// androidStringsXML mirrors the subset of an android strings.xml resource file needed to check
// for blank translations.
type androidStringsXML struct {
	Strings []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:",chardata"`
	} `xml:"string"`
}

// checkAndroidBlankTranslations looks for blank (or whitespace-only) string values in an android
// strings.xml payload, which usually indicates a source error in loco. It prints the locale
// (resource directory) and key where it found the problem.
func checkAndroidBlankTranslations(locale string, xmlData []byte) {
	var strs androidStringsXML
	if err := xml.Unmarshal(xmlData, &strs); err != nil {
		slog.Error("error unmarshaling android strings xml for blank check", slog.Any("err", err))
		return
	}
	for _, s := range strs.Strings {
		if strings.TrimSpace(s.Value) == "" {
			fmt.Printf("blank translation found: locale=%s key=%s\n", locale, s.Name)
		}
	}
}
