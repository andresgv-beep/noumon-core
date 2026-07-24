package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	studioSchemaVersion  = 1
	studioMaxRequest     = 2 << 20
	studioMaxBlocks      = 1000
	studioMaxBlockDepth  = 4
	studioMaxTextRunes   = 1 << 20
	studioMaxTags        = 50
	studioMaxFacetValues = 32
)

var (
	errStudioNotFound         = errors.New("studio document not found")
	errStudioForbidden        = errors.New("studio document forbidden")
	errStudioConflict         = errors.New("studio revision conflict")
	errStudioRevisionNotFound = errors.New("studio revision not found")
	errStudioAssetInvalid     = errors.New("studio asset invalid")

	studioSlugRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
	studioIDRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

var studioTemplates = map[string]string{
	"document":        "documents",
	"technical":       "documents",
	"story":           "documents",
	"cabinet.pdf":     "cabinet",
	"cabinet.reader":  "cabinet",
	"cabinet.gallery": "cabinet",
	"cabinet.audio":   "cabinet",
	"cabinet.video":   "cabinet",
	"moments.video":   "moments",
}

var studioBlockTypes = map[string]bool{
	"paragraph": true, "heading": true, "bulletList": true, "orderedList": true,
	"quote": true, "image": true, "table": true, "code": true, "callout": true,
	"divider": true, "columns": true, "itemRef": true,
}

type StudioClassification struct {
	WorkType    string   `json:"workType,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	Audience    []string `json:"audience,omitempty"`
	SeriesID    string   `json:"seriesId,omitempty"`
	SeriesTitle string   `json:"seriesTitle,omitempty"`
	Position    int      `json:"position,omitempty"`
}

type StudioPresentation struct {
	ContentWidth string `json:"contentWidth,omitempty"`
	FontPreset   string `json:"fontPreset,omitempty"`
}

type StudioContent struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Classification StudioClassification `json:"classification,omitempty"`
	Presentation   StudioPresentation   `json:"presentation,omitempty"`
	Blocks         []json.RawMessage    `json:"blocks"`
}

type StudioDocumentInput struct {
	TemplateKey  string          `json:"templateKey"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary,omitempty"`
	Language     string          `json:"language,omitempty"`
	AuthorLabel  string          `json:"authorLabel,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	Content      json.RawMessage `json:"content"`
	BaseRevision int             `json:"baseRevision,omitempty"`
}

type StudioDocument struct {
	ID                string               `json:"id"`
	OwnerUserID       *int64               `json:"ownerUserId,omitempty"`
	TemplateKey       string               `json:"templateKey"`
	Status            string               `json:"status"`
	Title             string               `json:"title"`
	Summary           string               `json:"summary,omitempty"`
	Language          string               `json:"language,omitempty"`
	AuthorLabel       string               `json:"authorLabel,omitempty"`
	Tags              []string             `json:"tags"`
	Classification    StudioClassification `json:"classification"`
	Metadata          json.RawMessage      `json:"metadata"`
	Content           json.RawMessage      `json:"content"`
	CoverAssetID      string               `json:"coverAssetId,omitempty"`
	Revision          int                  `json:"revision"`
	PublishedRevision *int                 `json:"publishedRevision,omitempty"`
	PublicationKind   string               `json:"publicationKind,omitempty"`
	PublicationTarget string               `json:"publicationTarget,omitempty"`
	Created           int64                `json:"created"`
	Updated           int64                `json:"updated"`
	Published         *int64               `json:"published,omitempty"`
}

type studioValidatedInput struct {
	Input          StudioDocumentInput
	Content        StudioContent
	Classification StudioClassification
	PlainText      string
	Links          []string
	Assets         []string
	Facets         map[string][]string
}

type StudioPortableSnapshot struct {
	FormatVersion  int                  `json:"formatVersion"`
	ContentID      string               `json:"contentId"`
	TemplateKey    string               `json:"templateKey"`
	Title          string               `json:"title"`
	Summary        string               `json:"summary,omitempty"`
	Language       string               `json:"language,omitempty"`
	AuthorLabel    string               `json:"authorLabel,omitempty"`
	Tags           []string             `json:"tags,omitempty"`
	Classification StudioClassification `json:"classification"`
	Metadata       json.RawMessage      `json:"metadata"`
	Content        json.RawMessage      `json:"content"`
}

func newStudioID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// UUIDv4 layout, encoded without punctuation so it is safe in URL segments.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:]), nil
}

func studioSurfaceForTemplate(template string) (string, bool) {
	surface, ok := studioTemplates[template]
	return surface, ok
}

func validateStudioInput(in StudioDocumentInput) (studioValidatedInput, error) {
	in.TemplateKey = strings.TrimSpace(in.TemplateKey)
	if _, ok := studioSurfaceForTemplate(in.TemplateKey); !ok {
		return studioValidatedInput{}, fmt.Errorf("templateKey: unsupported")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Summary = strings.TrimSpace(in.Summary)
	in.Language = strings.TrimSpace(in.Language)
	in.AuthorLabel = strings.TrimSpace(in.AuthorLabel)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 240 {
		return studioValidatedInput{}, fmt.Errorf("title: required or too long")
	}
	if utf8.RuneCountInString(in.Summary) > 2000 ||
		utf8.RuneCountInString(in.AuthorLabel) > 240 ||
		utf8.RuneCountInString(in.Language) > 32 {
		return studioValidatedInput{}, fmt.Errorf("metadata: value too long")
	}
	tags, err := normalizeStudioLabels(in.Tags, studioMaxTags, 64, false)
	if err != nil {
		return studioValidatedInput{}, fmt.Errorf("tags: %w", err)
	}
	in.Tags = tags
	if len(in.Metadata) == 0 || bytes.Equal(bytes.TrimSpace(in.Metadata), []byte("null")) {
		in.Metadata = json.RawMessage(`{}`)
	}
	if len(in.Metadata) > 128<<10 || !json.Valid(in.Metadata) {
		return studioValidatedInput{}, fmt.Errorf("metadata: invalid")
	}
	var metadataObject map[string]any
	if err := json.Unmarshal(in.Metadata, &metadataObject); err != nil {
		return studioValidatedInput{}, fmt.Errorf("metadata: object required")
	}
	mediaMetadata, mediaAssets, err := validateStudioMediaMetadata(in.TemplateKey, in.Metadata)
	if err != nil {
		return studioValidatedInput{}, err
	}
	if strings.HasPrefix(in.TemplateKey, "cabinet.") || in.TemplateKey == "moments.video" {
		in.Metadata, err = json.Marshal(mediaMetadata)
		if err != nil {
			return studioValidatedInput{}, err
		}
	}

	if len(in.Content) == 0 || len(in.Content) > studioMaxRequest || !json.Valid(in.Content) {
		return studioValidatedInput{}, fmt.Errorf("content: invalid or too large")
	}
	var content StudioContent
	if err := json.Unmarshal(in.Content, &content); err != nil {
		return studioValidatedInput{}, fmt.Errorf("content: %w", err)
	}
	if content.SchemaVersion != studioSchemaVersion {
		return studioValidatedInput{}, fmt.Errorf("schemaVersion: unsupported")
	}
	if len(content.Blocks) > studioMaxBlocks {
		return studioValidatedInput{}, fmt.Errorf("blocks: too many")
	}
	switch content.Presentation.ContentWidth {
	case "", "reading", "wide", "compact", "editorial":
	default:
		return studioValidatedInput{}, fmt.Errorf("presentation.contentWidth: invalid")
	}
	switch content.Presentation.FontPreset {
	case "", "editorial", "sans":
	default:
		return studioValidatedInput{}, fmt.Errorf("presentation.fontPreset: invalid")
	}
	classification, facets, err := validateStudioClassification(content.Classification)
	if err != nil {
		return studioValidatedInput{}, err
	}
	content.Classification = classification

	state := studioBlockValidation{
		ids: map[string]bool{}, links: map[string]bool{}, assets: map[string]bool{},
	}
	for _, raw := range content.Blocks {
		if err := state.validate(raw, 0); err != nil {
			return studioValidatedInput{}, err
		}
	}
	if state.count > studioMaxBlocks || state.runes > studioMaxTextRunes {
		return studioValidatedInput{}, fmt.Errorf("content: limits exceeded")
	}
	plain := strings.TrimSpace(strings.Join(state.plain, "\n"))
	links := make([]string, 0, len(state.links))
	for id := range state.links {
		links = append(links, id)
	}
	sort.Strings(links)
	assets := make([]string, 0, len(state.assets))
	for id := range state.assets {
		assets = append(assets, id)
	}
	for _, id := range mediaAssets {
		if !state.assets[id] {
			assets = append(assets, id)
		}
	}
	sort.Strings(assets)
	normalizedContent, err := json.Marshal(content)
	if err != nil {
		return studioValidatedInput{}, err
	}
	in.Content = normalizedContent
	return studioValidatedInput{
		Input: in, Content: content, Classification: classification,
		PlainText: plain, Links: links, Assets: assets, Facets: facets,
	}, nil
}

func validateStudioClassification(c StudioClassification) (StudioClassification, map[string][]string, error) {
	var err error
	if c.WorkType, err = normalizeStudioSlug(c.WorkType); err != nil {
		return c, nil, fmt.Errorf("classification.workType: %w", err)
	}
	if c.SeriesID, err = normalizeStudioSlug(c.SeriesID); err != nil {
		return c, nil, fmt.Errorf("classification.seriesId: %w", err)
	}
	c.SeriesTitle = strings.TrimSpace(c.SeriesTitle)
	if utf8.RuneCountInString(c.SeriesTitle) > 240 || c.Position < 0 {
		return c, nil, fmt.Errorf("classification.series: invalid")
	}
	if c.Topics, err = normalizeStudioLabels(c.Topics, studioMaxFacetValues, 64, true); err != nil {
		return c, nil, fmt.Errorf("classification.topics: %w", err)
	}
	if c.Audience, err = normalizeStudioLabels(c.Audience, studioMaxFacetValues, 64, true); err != nil {
		return c, nil, fmt.Errorf("classification.audience: %w", err)
	}
	facets := map[string][]string{}
	if c.WorkType != "" {
		facets["workType"] = []string{c.WorkType}
	}
	if len(c.Topics) > 0 {
		facets["topic"] = append([]string(nil), c.Topics...)
	}
	if len(c.Audience) > 0 {
		facets["audience"] = append([]string(nil), c.Audience...)
	}
	if c.SeriesID != "" {
		facets["series"] = []string{c.SeriesID}
	}
	return c, facets, nil
}

func normalizeStudioSlug(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", nil
	}
	if !studioSlugRE.MatchString(v) {
		return "", fmt.Errorf("invalid slug")
	}
	return v, nil
}

func normalizeStudioLabels(values []string, maxItems, maxRunes int, slugs bool) ([]string, error) {
	if len(values) > maxItems {
		return nil, fmt.Errorf("too many values")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if slugs {
			var err error
			value, err = normalizeStudioSlug(value)
			if err != nil {
				return nil, err
			}
		}
		if value == "" {
			continue
		}
		if utf8.RuneCountInString(value) > maxRunes {
			return nil, fmt.Errorf("value too long")
		}
		key := strings.ToLower(value)
		if !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out, nil
}

type studioBlockValidation struct {
	ids    map[string]bool
	links  map[string]bool
	assets map[string]bool
	count  int
	runes  int
	plain  []string
}

func (s *studioBlockValidation) validate(raw json.RawMessage, depth int) error {
	if depth > studioMaxBlockDepth {
		return fmt.Errorf("blocks: nesting too deep")
	}
	s.count++
	if s.count > studioMaxBlocks {
		return fmt.Errorf("blocks: too many")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return fmt.Errorf("block: object required")
	}
	var typ, id string
	if err := json.Unmarshal(obj["type"], &typ); err != nil || !studioBlockTypes[typ] {
		return fmt.Errorf("block.type: unsupported")
	}
	if err := json.Unmarshal(obj["id"], &id); err != nil || !studioIDRE.MatchString(id) || s.ids[id] {
		return fmt.Errorf("block.id: invalid or duplicate")
	}
	s.ids[id] = true
	if typ == "heading" {
		var level int
		if err := json.Unmarshal(obj["level"], &level); err != nil || level < 1 || level > 3 {
			return fmt.Errorf("heading.level: invalid")
		}
	}
	for _, key := range []string{"text", "caption", "alt", "title", "titleSnapshot"} {
		if field, ok := obj[key]; ok {
			var text string
			if err := json.Unmarshal(field, &text); err != nil {
				return fmt.Errorf("block.%s: string required", key)
			}
			n := utf8.RuneCountInString(text)
			if n > 100000 {
				return fmt.Errorf("block.%s: too long", key)
			}
			s.runes += n
			if strings.TrimSpace(text) != "" && key != "alt" {
				s.plain = append(s.plain, strings.TrimSpace(text))
			}
		}
	}
	if field, ok := obj["items"]; ok {
		var items []string
		if err := json.Unmarshal(field, &items); err != nil || len(items) > 500 {
			return fmt.Errorf("block.items: invalid")
		}
		for _, item := range items {
			if utf8.RuneCountInString(item) > 10000 {
				return fmt.Errorf("block.items: value too long")
			}
			s.runes += utf8.RuneCountInString(item)
			s.plain = append(s.plain, strings.TrimSpace(item))
		}
	}
	if typ == "image" {
		var assetID string
		if err := json.Unmarshal(obj["assetId"], &assetID); err != nil || !studioIDRE.MatchString(assetID) {
			return fmt.Errorf("image.assetId: invalid")
		}
		s.assets[assetID] = true
	}
	if typ == "itemRef" {
		var itemID string
		if err := json.Unmarshal(obj["itemId"], &itemID); err != nil || !studioIDRE.MatchString(itemID) {
			return fmt.Errorf("itemRef.itemId: invalid")
		}
		s.links[itemID] = true
	}
	if typ == "table" {
		if err := s.validateTable(obj["rows"]); err != nil {
			return err
		}
	}
	for _, key := range []string{"blocks", "children"} {
		if field, ok := obj[key]; ok {
			var children []json.RawMessage
			if err := json.Unmarshal(field, &children); err != nil {
				return fmt.Errorf("block.%s: array required", key)
			}
			for _, child := range children {
				if err := s.validate(child, depth+1); err != nil {
					return err
				}
			}
		}
	}
	if field, ok := obj["columns"]; ok {
		var columns [][]json.RawMessage
		if err := json.Unmarshal(field, &columns); err != nil || len(columns) < 1 || len(columns) > 3 {
			return fmt.Errorf("columns: one to three columns required")
		}
		for _, column := range columns {
			for _, child := range column {
				if err := s.validate(child, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *studioBlockValidation) validateTable(raw json.RawMessage) error {
	var rows [][]string
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) > 100 {
		return fmt.Errorf("table.rows: invalid")
	}
	for _, row := range rows {
		if len(row) > 20 {
			return fmt.Errorf("table.rows: too many columns")
		}
		for _, cell := range row {
			n := utf8.RuneCountInString(cell)
			if n > 10000 {
				return fmt.Errorf("table.cell: too long")
			}
			s.runes += n
			s.plain = append(s.plain, strings.TrimSpace(cell))
		}
	}
	return nil
}

func studioPortableSnapshot(doc StudioDocument) StudioPortableSnapshot {
	return StudioPortableSnapshot{
		FormatVersion: 1, ContentID: doc.ID, TemplateKey: doc.TemplateKey,
		Title: doc.Title, Summary: doc.Summary, Language: doc.Language,
		AuthorLabel: doc.AuthorLabel, Tags: append([]string(nil), doc.Tags...),
		Classification: doc.Classification,
		Metadata:       append(json.RawMessage(nil), doc.Metadata...),
		Content:        append(json.RawMessage(nil), doc.Content...),
	}
}
