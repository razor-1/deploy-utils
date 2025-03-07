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
	"regexp"
)

const (
	androidURL        = locoBaseURL + "/export/archive/xml.zip"
	locoAndroidFormat = "android"
)

var (
	// special things we need to do to get the proper output directory names
	androidLocaleMap = map[string]string{
		"pl-PL":   "pl",
		"sv-SE":   "sv",
		"da-DK":   "da",
		"lt-LT":   "lt",
		"ko-KR":   "ko",
		"cs-CZ":   "cs",
		"hr-HR":   "hr",
		"bg-BG":   "bg",
		"ja-JP":   "ja",
		"ro-RO":   "ro",
		"zh-CN":   "zh",
		"uk-UA":   "uk",
		"hu-HU":   "hu",
		"el-GR":   "el",
		"vi-VN":   "vi",
		"th-TH":   "th",
		"fi-FI":   "fi",
		"gu-IN":   "gu",
		"id-ID":   "in", // java is really cool and uses "in" for indonesian
		"tr-TR":   "tr",
		"zh-Hant": "b+zh+Hant", // this is the android BCP 47 thing
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

		outputDir := filepath.Join(baseDir, filepath.Base(dir))
		if !isValidDir(outputDir) {
			// output directory doesn't exist. we might need to map it
			matches := androidResourceRegex.FindStringSubmatch(filepath.Base(dir))
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
					slog.String("filename", zipFile.Name))
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
