package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

type XCodeAsset struct {
	ExtractionState string                    `json:"extractionState"`
	Localizations   map[string]map[string]any `json:"localizations"`
}

type XCodeStrings struct {
	SourceLanguage string                `json:"sourceLanguage"`
	Strings        map[string]XCodeAsset `json:"strings"`
	Version        string                `json:"version"`
}

const (
	XcStrings              = "xcstrings"
	iosCatalogURLTemplate  = locoBaseURL + "/export/all." + XcStrings
	stringsCatalogFilename = "Localizable." + XcStrings
	plistCatalogFilename   = "InfoPlist." + XcStrings
	plistTagName           = "ios-plist"
	extractionStateManual  = "manual"
	bundleNameAsset        = "CFBundleName"
)

var (
	iosFilters = []string{
		"ios-strings,ios-plurals", plistTagName,
	}
)

func updateiOSAssetsCatalog(apiKey, baseDir string) error {
	// verify that baseDir is valid
	if !isValidDir(baseDir) {
		return fmt.Errorf("invalid base dir: %s", baseDir)
	}

	getTranslations := func(filter string) (*http.Response, error) {
		qp := url.Values{}
		qp.Set(locoFilter, filter)
		qp.Set("index", "id")
		qp.Set("fallback", locoFallback)

		resp, err := locoRequest(apiKey, iosCatalogURLTemplate, qp)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(iosFilters))
	successCount := atomic.Int32{}
	for _, f := range iosFilters {
		filter := f
		go func() {
			defer wg.Done()
			resp, err := getTranslations(filter)
			if err != nil || resp.StatusCode != http.StatusOK {
				slog.Error("error getting", slog.String("filter", filter),
					slog.Int("status", resp.StatusCode), slog.Any("err", err.Error()))
			} else {
				err = processTranslationsCatalog(filter, baseDir, resp)
				if err != nil {
					slog.Error("error processing", slog.String("filter", filter),
						slog.Any("err", err.Error()))
				} else {
					successCount.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if int(successCount.Load()) != len(iosFilters) {
		return fmt.Errorf("did not process %d sets as expected", len(iosFilters))
	}

	return nil
}

func processTranslationsCatalog(filter, baseDir string, resp *http.Response) error {
	defer resp.Body.Close()

	var catalog XCodeStrings
	err := json.NewDecoder(resp.Body).Decode(&catalog)
	if err != nil {
		return err
	}
	// basically this changes "en-US" to "en"
	catalog.SourceLanguage = locales[catalog.SourceLanguage]

	isPlist := filter == plistTagName
	skippedAndLogged := make(map[string]bool)

	// change the locale to be what we need
	assetsToDelete := make(map[string]struct{}, len(plistAssetMap))
	for asset, locs := range catalog.Strings {
		locsToDelete := make([]string, 0)
		locs.ExtractionState = extractionStateManual
		for rawLocale, v := range locs.Localizations {
			locale := iosLocale(rawLocale)
			if locale != rawLocale {
				catalog.Strings[asset].Localizations[locale] = v
				locsToDelete = append(locsToDelete, rawLocale)
			}
			// if the lproj directory doesn't exist for this locale, then add it to the remove list; we don't want it
			if skippedAndLogged[locale] {
				locsToDelete = append(locsToDelete, locale)
			} else if !validiOSLocales[locale] {
				slog.Info("skipping", slog.String("locale", locale))
				locsToDelete = append(locsToDelete, locale)
				skippedAndLogged[locale] = true
			}
		}
		for _, loc := range locsToDelete {
			delete(catalog.Strings[asset].Localizations, loc)
		}
		for _, plKey := range plistAssetMap[asset] {
			catalog.Strings[plKey] = catalog.Strings[asset]
			if plKey == bundleNameAsset {
				checkBundleNameLength(catalog.Strings[plKey].Localizations)
			}
			if plKey != asset {
				assetsToDelete[asset] = struct{}{}
			}
		}
	}

	for assetToDelete := range assetsToDelete {
		delete(catalog.Strings, assetToDelete)
	}

	outputFilename := stringsCatalogFilename
	if isPlist {
		outputFilename = plistCatalogFilename
	}
	outputPath := filepath.Join(baseDir, outputFilename)
	outFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer outFile.Close()
	err = json.NewEncoder(outFile).Encode(catalog)
	return err
}

func checkBundleNameLength(localizations map[string]map[string]any) {
	for locale, valMap := range localizations {
		if stringUnit, ok := valMap["stringUnit"]; ok {
			if suMap, ok := stringUnit.(map[string]any); ok {
				valInt := suMap["value"]
				var value string
				value, _ = valInt.(string)
				if len(value) == 0 || len(value) > 15 {
					slog.Warn(bundleNameAsset+" too long", slog.String("locale", locale),
						slog.Int("length", len(value)))
				}
			}
		}
	}
}

var validiOSLocales = map[string]bool{
	"en":      true,
	"pt":      true,
	"es":      true,
	"it":      true,
	"nl":      true,
	"de":      true,
	"fr":      true,
	"pl":      true,
	"sv":      true,
	"pt-PT":   true,
	"da":      true,
	"sw":      true,
	"lt":      true,
	"ko":      true,
	"ru":      true,
	"cs":      true,
	"hr":      true,
	"ja":      true,
	"bg":      true,
	"ro":      true,
	"hu":      true,
	"uk":      true,
	"el":      true,
	"vi":      true,
	"th":      true,
	"et":      true,
	"fi":      true,
	"id":      true,
	"ht":      true,
	"kea":     true,
	"sl":      true,
	"fil":     true,
	"gu":      true,
	"tr":      true,
	"sq":      true,
	"zh-Hans": true,
	"sk":      true,
	"zh-Hant": true,
	"af":      true,
	"ee":      true,
	"vec":     true,
	"ca":      true,
	"gl":      true,
	"si":      true,
	"jam":     true,
	"hy":      true,
	"ms":      true,
	"ka":      true,
	"az":      true,
	"ne":      true,
	"fon":     true,
	"ak":      true,
	"mfe":     true,
	"sr-Latn": true,
}
