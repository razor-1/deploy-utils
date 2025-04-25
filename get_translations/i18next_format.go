package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

type LocoTranslationBase struct {
	ID          string                `json:"id"`
	Translated  bool                  `json:"translated"`
	Translation string                `json:"translation"`
	Plurals     []LocoTranslationBase `json:"plurals"`
}

type LocoTranslation struct {
	LocoTranslationBase
	Locale LocoLocale `json:"locale"`
}

type LocoAssetPrintf struct {
	// 	ID      string `json:"id"`
	// 	Type    string `json:"type"`
	// 	Context string `json:"context"`
	// 	Notes   string `json:"notes"`
	Printf string `json:"printf"`
}

const (
	locoJsonTranslationsURL = locoBaseURL + "/translations/%s.json"
	locoPostTranslationURL  = locoBaseURL + "/translations/%s/%s"
	locoPatchAssetURL       = locoBaseURL + "/assets/%s.json"
)

func i18nextConvertFormat(apiKey, assetID, formatKey string) error {
	if formatKey == "" {
		// parse it from the asset ID
		formatKey = parseFormatKey(assetID)
		if formatKey == "" {
			return errors.New("couldn't determine format key")
		}
	}
	escapedAssetID := url.PathEscape(assetID)

	// get all the translations
	resp, err := locoRequest(apiKey, fmt.Sprintf(locoJsonTranslationsURL, escapedAssetID), url.Values{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var translations []LocoTranslation
	jd := json.NewDecoder(resp.Body)
	err = jd.Decode(&translations)
	if err != nil {
		return err
	}
	updateCount := 0
	for _, translation := range translations {
		if !translation.Translated {
			continue
		}
		newTranslation := pythonToI18Next(translation.Translation, formatKey)
		if newTranslation == translation.Translation || newTranslation == "" {
			slog.Warn("could not create translation", slog.String("locale", translation.Locale.Code),
				slog.String("translation", newTranslation))
			continue
		}

		transURL := fmt.Sprintf(locoPostTranslationURL, escapedAssetID, translation.Locale.Code)
		_, newErr := locoWrite(apiKey, transURL, http.MethodPost, []byte(newTranslation))
		if newErr != nil {
			slog.Error("failed to write translation", slog.Any("err", newErr))
		} else {
			updateCount++
			if strings.HasPrefix(translation.Locale.Code, "ca") {
				slog.Info("ca translation debug", transURL,
					slog.String("localeCode", translation.Locale.Code),
					slog.String("localeName", translation.Locale.Name),
				)
			}
		}
	}

	if updateCount > 0 {
		// update the asset to have the i18next format type
		updateAssetURL := fmt.Sprintf(locoPatchAssetURL, escapedAssetID)
		printfPatch := LocoAssetPrintf{Printf: "i18next"}
		body, _ := json.Marshal(printfPatch)
		_, updErr := locoWrite(apiKey, updateAssetURL, http.MethodPatch, body)
		if updErr != nil {
			slog.Error("failed to update asset printf", slog.Any("err", updErr))
		}
	}

	slog.Info("updated translations", slog.Int("count", updateCount))
	return nil
}

func parseFormatKey(assetID string) string {
	fkRegex := regexp.MustCompile(`%\(([\w-]+)\)s`)
	matches := fkRegex.FindStringSubmatch(assetID)
	if len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func pythonToI18Next(translation, formatKey string) string {
	pythonFmt := "%(" + formatKey + ")s"
	i18nextFormat := fmt.Sprintf("{{%s}}", formatKey)
	return strings.ReplaceAll(translation, pythonFmt, i18nextFormat)
}
