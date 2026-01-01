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
)

const (
	locoPOExportURL = locoBaseURL + "/export/archive/po.zip"
	backendTag      = "backend"
)

func getPOExport(apiKey string, args []string) error {
	qp := url.Values{}
	qp.Add("index", "name")
	qp.Add(locoFilter, backendTag)
	qp.Add("fallback", "en-US")

	resp, err := locoRequest(apiKey, locoPOExportURL, qp)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return writeLocoPO(args[0], resp.Body)
}

func writeLocoPO(baseDir string, zipData io.ReadCloser) error {
	body, err := io.ReadAll(zipData)
	if err != nil {
		return fmt.Errorf("error reading all response bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("zip.NewReader error: %v", err)
	}

	var readErr error
	for _, zipFile := range zipReader.File {
		zipPath, zipName := filepath.Split(zipFile.Name)
		ext := filepath.Ext(zipName)
		if ext != ".po" {
			continue
		}

		poDir, err := outputFromZip(baseDir, zipPath, zipFile)
		if err != nil {
			readErr = err
		}
		if poDir == "" {
			// skip
			continue
		}

		localeCode := localeFromPath(poDir)
		l, err := language.Parse(localeCode)
		if err != nil {
			slog.Error("language.Parse failed for",
				slog.String("locale", localeCode), slog.Any("err", err))
			continue
		}
		if l.String() != localeCode {
			// go's language parsing ends up with a different code than we expect. copy the file out so that we have both.
			slog.Info("mismatch for code", slog.String("locale", localeCode),
				slog.String("locStr", l.String()))
			newPath := strings.Replace(zipPath, fmt.Sprintf("/%s/", localeCode),
				fmt.Sprintf("/%s/", l.String()), 1)
			poFile, _, err := createOutputFile(baseDir, newPath, true)
			if err != nil {
				slog.Error("error creating dup output file for",
					slog.String("loc", l.String()), slog.Any("err", err))
				continue
			}
			_, err = writeZipFile(zipFile, poFile)
			if err != nil && err != io.EOF {
				slog.Error("error creating dup output file for",
					slog.String("loc", l.String()), slog.Any("err", err))
			}
			poFile.Close()
		}
	}

	return readErr
}

func localeFromPath(dir string) string {
	parts := strings.Split(dir, "/")
	if len(parts) < 2 {
		return ""
	}
	localePart := parts[len(parts)-2]
	// the path uses '@' to indicate the script
	atSplit := strings.Split(localePart, "@")
	if len(atSplit) > 1 {
		return fmt.Sprintf("%s-%s", atSplit[0], strings.Title(atSplit[1]))
	}
	return localePart
}

func outputFromZip(baseDir, zipPath string, zipFile *zip.File) (poDir string, err error) {
	poFile, poDir, err := createOutputFile(baseDir, zipPath, true)
	if err != nil {
		return
	}
	if poFile == nil {
		// this happens when we skip something - not an error, but e.g. a locale we don't want to process
		return poDir, nil
	}
	defer poFile.Close()

	_, err = writeZipFile(zipFile, poFile)
	if err != nil && err != io.EOF {
		slog.Error("error writing contents to po file",
			slog.String("file", poFile.Name()), slog.Any("err", err))
		return
	}

	return poDir, nil
}

// writeZipFile takes the compressed po file from the archive and writes it
func writeZipFile(zf *zip.File, out io.Writer) (int64, error) {
	f, err := zf.Open()
	if err != nil {
		return 0, err
	}
	defer f.Close()
	// max 10MB
	return io.CopyN(out, f, 10*1000000)
}

func createOutputFile(baseDir, zipPath string, noskip bool) (poFile *os.File, poDir string, err error) {
	components := strings.Split(zipPath, "/")
	if len(components) < 3 {
		err = fmt.Errorf("path length for %s is not expected", zipPath)
		return
	}
	// change from en_US to en-US, for example
	locale := strings.Replace(components[2], "_", "-", 1)
	localeDir, ok := locales[locale]
	if !ok {
		if !noskip {
			slog.Info("skipping locale not in locales map", slog.String("locale", locale))
			return
		} else {
			localeDir = locale
		}
	}
	poDir = filepath.Join(baseDir, localeDir, "LC_MESSAGES")
	err = os.MkdirAll(poDir, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("error creating output directory: %w", err)
		return
	}

	poFilename := filepath.Join(poDir, "messages.po")
	poFile, err = os.Create(poFilename)
	if err != nil {
		err = fmt.Errorf("cannot create output po file: %w", err)
		return
	}

	return
}
