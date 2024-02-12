package main

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	"net/url"
	"slices"
	"strings"
)

const (
	locoLocales = "https://localise.biz/api/locales"
)

type LocoLocale struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Source bool   `json:"source"`
}

func getFallbackLangs(apiKey string) error {
	resp, err := locoRequest(apiKey, locoLocales, url.Values{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	jd := json.NewDecoder(resp.Body)
	var allLocales []LocoLocale
	err = jd.Decode(&allLocales)
	if err != nil {
		log.Errorf("error reading response: %v", err)
		return err
	}

	supported := make([]language.Tag, 0, len(allLocales))
	var sourceTag language.Tag
	// add the source locale first
	for _, loc := range allLocales {
		if loc.Source {
			sourceTag = language.Make(loc.Code)
			supported = append(supported, sourceTag)
			break
		}
	}
	for _, loc := range allLocales {
		if loc.Source {
			continue
		}
		supported = append(supported, language.Make(loc.Code))
	}

	// we now have the supported list. let's go get the matcher list for each
	for _, sup := range supported {
		if tagsMatch(sup, sourceTag, false) {
			continue
		}

		matches := allMatches(supported, sup, sourceTag)
		fmt.Printf("%s: %s\n", sup.String(), strings.Join(tagsToString(matches), ", "))
	}

	return nil
}

func allMatches(supported []language.Tag, toMatch, source language.Tag) []language.Tag {
	matched := make([]language.Tag, 0)

	otherSupported := make([]language.Tag, len(supported))
	copy(otherSupported, supported)
	otherSupported = slices.DeleteFunc(otherSupported, func(tag language.Tag) bool {
		return tagsMatch(tag, toMatch, false)
	})
	match := langMatch(otherSupported, toMatch)
	matched = append(matched, match)
	if tagsMatch(match, source, false) {
		return matched
	}

	nextSupported := make([]language.Tag, len(otherSupported))
	copy(nextSupported, otherSupported)
	nextSupported = slices.DeleteFunc(nextSupported, func(tag language.Tag) bool {
		return tagsMatch(tag, match, false)
	})
	return append(matched, allMatches(nextSupported, toMatch, source)...)
}

func langMatch(supported []language.Tag, toMatch language.Tag) language.Tag {
	matcher := language.NewMatcher(supported)
	_, index, _ := matcher.Match(toMatch)
	return supported[index]
}

func tagsToString(tags []language.Tag) []string {
	tagStrings := make([]string, len(tags))
	for i, m := range tags {
		tagStrings[i] = m.String()
	}
	return tagStrings
}

func tagsMatch(t1, t2 language.Tag, baseOnly bool) bool {
	if t1 == t2 {
		return true
	}
	t1b, _ := t1.Base()
	t2b, _ := t2.Base()
	if t1b == t2b {
		if baseOnly {
			return true
		}
		t1r, _ := t1.Region()
		t2r, _ := t2.Region()
		return t1r == t2r
	}
	return false
}
