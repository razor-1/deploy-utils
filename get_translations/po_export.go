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

		poFile, err := createOutputFile(baseDir, zipPath)
		if err != nil {
			readErr = err
			continue
		}
		if poFile == nil {
			// this happens when we skip something - not an error, but e.g. a locale we don't want to process
			continue
		}

		_, err = writeZipFile(zipFile, poFile)
		_ = poFile.Close()
		if err != nil && err != io.EOF {
			log.Errorf("error writing contents to po file: %v", err)
			readErr = err
			continue
		}
	}

	return readErr
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

func createOutputFile(baseDir, zipPath string) (poFile *os.File, err error) {
	components := strings.Split(zipPath, "/")
	if len(components) < 3 {
		err = fmt.Errorf("path length for %s is not expected", zipPath)
		return
	}
	// change from en_US to en-US, for example
	locale := strings.Replace(components[2], "_", "-", 1)
	localeDir, ok := locales[locale]
	if !ok {
		log.Infof("skipping locale %s: not in locales map", locale)
		return
	}
	poDir := filepath.Join(baseDir, localeDir, "LC_MESSAGES")
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
