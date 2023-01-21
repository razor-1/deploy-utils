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
)

const (
	locoPOExportURL = locoBaseURL + "/export/archive/po.zip"
)

func getPOExport(apiKey string, args []string) error {
	qp := url.Values{}
	qp.Add("index", "name")
	qp.Add("filter", "web-pub")
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
		log.Fatalf("error reading all response bytes: %v", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		log.Fatalf("zip.NewReader error: %v", err)
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
			log.Errorf("language.Parse failed for %s: %v", localeCode, err)
			continue
		}
		if l.String() != localeCode {
			// go's language parsing ends up with a different code than we expect. copy the file out so that we have both.
			log.Infof("mismatch for code %s: %s", localeCode, l.String())
			newPath := strings.Replace(zipPath, fmt.Sprintf("/%s/", localeCode), fmt.Sprintf("/%s/", l.String()), 1)
			poFile, _, err := createOutputFile(baseDir, newPath, true)
			if err != nil {
				log.Errorf("error creating dup output file for %s: %v", l.String(), err)
				continue
			}
			_, err = writeZipFile(zipFile, poFile)
			if err != nil && err != io.EOF {
				log.Errorf("error writing dup output file for %s: %v", l.String(), err)
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
	return parts[len(parts)-2]
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
		log.Errorf("error writing contents to po file: %v", err)
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
			log.Infof("skipping locale %s: not in locales map", locale)
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
