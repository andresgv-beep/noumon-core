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
	studioSchemaVersion  = 2
	studioLegacySchema   = 1
	studioMaxRequest     = 2 << 20
	studioMaxBlocks      = 1000
	studioMaxBlockDepth  = 4
	studioMaxTextRunes   = 1 << 20
	studioMaxTags        = 50
	studioMaxFacetValues = 32
	studioMaxPages       = 100
	studioMaxInfoRows    = 40
	studioMaxInfoCards   = 4
)

var (
	errStudioNotFound             = errors.New("studio document not found")
	errStudioForbidden            = errors.New("studio document forbidden")
	errStudioConflict             = errors.New("studio revision conflict")
	errStudioRevisionNotFound     = errors.New("studio revision not found")
	errStudioAssetInvalid         = errors.New("studio asset invalid")
	errStudioPurgeRequiresArchive = errors.New("studio purge requires archived document")
	errStudioBrokenPageLinks      = errors.New("studio page links unresolved")

	studioSlugRE       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
	studioIDRE         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	studioInlineLinkRE = regexp.MustCompile(`\[\[(page|item):([A-Za-z0-9][A-Za-z0-9._:-]{0,127})\|([^\]\r\n]+)\]\]`)
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
	// Marco del menu de contenidos: vacio o "none" a ras, "framed" con marco
	// recto, "rounded" con esquinas redondeadas.
	NavFrame string `json:"navFrame,omitempty"`
	// Tamano del texto del menu en px; 0 = el valor por defecto del cliente.
	NavFontSize      int    `json:"navFontSize,omitempty"`
	FontPreset       string `json:"fontPreset,omitempty"`
	TitleFontSize    int    `json:"titleFontSize,omitempty"`
	SummaryFontSize  int    `json:"summaryFontSize,omitempty"`
	TitleTextAlign   string `json:"titleTextAlign,omitempty"`
	SummaryTextAlign string `json:"summaryTextAlign,omitempty"`
}

type StudioContent struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	Classification StudioClassification `json:"classification,omitempty"`
	Presentation   StudioPresentation   `json:"presentation,omitempty"`
	InfoCard       *StudioInfoCard      `json:"infoCard,omitempty"`
	Blocks         []json.RawMessage    `json:"blocks,omitempty"`
	Pages          []StudioPage         `json:"pages,omitempty"`
	// Titulo del menu de contenidos. Vacio = el nombre por defecto del cliente.
	NavTitle string `json:"navTitle,omitempty"`
}

type StudioInfoCard struct {
	// Identidad dentro de la página, lado del artículo al que se pega y bloque
	// junto al que empieza a flotar (0 = arriba del todo).
	ID      string `json:"id,omitempty"`
	Side    string `json:"side,omitempty"`
	Anchor  int    `json:"anchor,omitempty"`
	AssetID string `json:"assetId,omitempty"`
	Caption string `json:"caption,omitempty"`
	// Encuadre de la imagen: vacío o "natural" respeta la forma original; el
	// resto recorta a una proporción fija. ImageFocus* (0-100) es el punto de
	// la imagen que se conserva al recortar.
	ImageRatio  string          `json:"imageRatio,omitempty"`
	ImageFocusX *int            `json:"imageFocusX,omitempty"`
	ImageFocusY *int            `json:"imageFocusY,omitempty"`
	Rows        []StudioInfoRow `json:"rows,omitempty"`
}

var studioInfoRatios = map[string]bool{
	"natural": true, "wide": true, "square": true, "portrait": true, "tall": true,
}

var studioInfoSides = map[string]bool{"right": true, "left": true}

type StudioInfoRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type StudioPage struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Blocks []json.RawMessage `json:"blocks"`
	// Seccion del menu: una pagina estrena grupo cuando difiere de la anterior.
	Section string `json:"section,omitempty"`
	// Las fichas son de la página, no del documento: cada página tiene las
	// suyas y una página nueva nace sin ninguna.
	InfoCards []StudioInfoCard `json:"infoCards,omitempty"`
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
	Input           StudioDocumentInput
	Content         StudioContent
	Classification  StudioClassification
	PlainText       string
	Pages           []studioValidatedPage
	Links           []string
	BrokenPageLinks []string
	Assets          []string
	Facets          map[string][]string
}

type studioValidatedPage struct {
	ID        string
	Title     string
	PlainText string
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
	surface, ok := studioSurfaceForTemplate(in.TemplateKey)
	if !ok {
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
	content, err = normalizeStudioContentVersion(surface, in.Title, content)
	if err != nil {
		return studioValidatedInput{}, err
	}
	switch content.Presentation.ContentWidth {
	case "", "reading", "wide", "compact", "editorial":
	default:
		return studioValidatedInput{}, fmt.Errorf("presentation.contentWidth: invalid")
	}
	switch content.Presentation.NavFrame {
	case "", "none", "framed", "rounded":
	default:
		return studioValidatedInput{}, fmt.Errorf("presentation.navFrame: invalid")
	}
	switch content.Presentation.FontPreset {
	case "", "editorial", "sans":
	default:
		return studioValidatedInput{}, fmt.Errorf("presentation.fontPreset: invalid")
	}
	if size := content.Presentation.NavFontSize; size != 0 && (size < 9 || size > 20) {
		return studioValidatedInput{}, fmt.Errorf("presentation.navFontSize: invalid")
	}
	if size := content.Presentation.TitleFontSize; size != 0 && (size < 10 || size > 96) {
		return studioValidatedInput{}, fmt.Errorf("presentation.titleFontSize: invalid")
	}
	if size := content.Presentation.SummaryFontSize; size != 0 && (size < 10 || size > 96) {
		return studioValidatedInput{}, fmt.Errorf("presentation.summaryFontSize: invalid")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"titleTextAlign", content.Presentation.TitleTextAlign},
		{"summaryTextAlign", content.Presentation.SummaryTextAlign},
	} {
		if field.value != "" && field.value != "left" && field.value != "center" && field.value != "right" {
			return studioValidatedInput{}, fmt.Errorf("presentation.%s: invalid", field.name)
		}
	}
	classification, facets, err := validateStudioClassification(content.Classification)
	if err != nil {
		return studioValidatedInput{}, err
	}
	content.Classification = classification

	state := studioBlockValidation{
		ids: map[string]bool{}, links: map[string]bool{}, assets: map[string]bool{},
		brokenPageLinks: map[string]bool{},
	}
	validatedPages := []studioValidatedPage{}
	content.NavTitle = strings.TrimSpace(content.NavTitle)
	if utf8.RuneCountInString(content.NavTitle) > 120 {
		return studioValidatedInput{}, fmt.Errorf("navTitle: too long")
	}
	if content.SchemaVersion == studioSchemaVersion {
		if len(content.Pages) < 1 || len(content.Pages) > studioMaxPages {
			return studioValidatedInput{}, fmt.Errorf("pages: one to %d pages required", studioMaxPages)
		}
		state.pageIDs = map[string]bool{}
		// Primero se recogen todas las identidades. Así un enlace puede apuntar
		// hacia una página posterior sin depender del orden del documento.
		for index := range content.Pages {
			page := &content.Pages[index]
			page.ID = strings.TrimSpace(page.ID)
			page.Title = strings.TrimSpace(page.Title)
			if !studioIDRE.MatchString(page.ID) || state.pageIDs[page.ID] {
				return studioValidatedInput{}, fmt.Errorf("page.id: invalid or duplicate")
			}
			if page.Title == "" || utf8.RuneCountInString(page.Title) > 240 {
				return studioValidatedInput{}, fmt.Errorf("page.title: required or too long")
			}
			// La seccion es opcional y agrupa paginas en el menu; vacia significa
			// que la pagina sigue en el grupo anterior.
			page.Section = strings.TrimSpace(page.Section)
			if utf8.RuneCountInString(page.Section) > 120 {
				return studioValidatedInput{}, fmt.Errorf("page.section: too long")
			}
			if page.Section != "" {
				state.runes += utf8.RuneCountInString(page.Section)
				state.plain = append(state.plain, page.Section)
			}
			state.pageIDs[page.ID] = true
		}
		for index := range content.Pages {
			page := &content.Pages[index]
			pagePlainStart := len(state.plain)
			state.runes += utf8.RuneCountInString(page.Title)
			state.plain = append(state.plain, page.Title)
			for _, raw := range page.Blocks {
				if err := state.validate(raw, 0); err != nil {
					return studioValidatedInput{}, err
				}
			}
			if len(page.InfoCards) > 0 && surface != "documents" {
				return studioValidatedInput{}, fmt.Errorf("page.infoCards: unsupported for surface")
			}
			if len(page.InfoCards) > studioMaxInfoCards {
				return studioValidatedInput{}, fmt.Errorf("page.infoCards: at most %d per page", studioMaxInfoCards)
			}
			cardIDs := make(map[string]bool, len(page.InfoCards))
			for cardIndex := range page.InfoCards {
				card := &page.InfoCards[cardIndex]
				card.ID = strings.TrimSpace(card.ID)
				if !studioIDRE.MatchString(card.ID) || cardIDs[card.ID] {
					return studioValidatedInput{}, fmt.Errorf("infoCard.id: invalid or duplicate")
				}
				cardIDs[card.ID] = true
				card.Side = strings.TrimSpace(card.Side)
				if card.Side == "" {
					card.Side = "right"
				}
				if !studioInfoSides[card.Side] {
					return studioValidatedInput{}, fmt.Errorf("infoCard.side: invalid")
				}
				// El anclaje es una posición, no una referencia: se recorta al
				// número de bloques en vez de rechazar el documento, porque los
				// bloques pueden haber desaparecido desde que se ancló.
				if card.Anchor < 0 {
					card.Anchor = 0
				}
				if last := len(page.Blocks) - 1; card.Anchor > last {
					card.Anchor = max(0, last)
				}
				if err := validateStudioInfoCard(card, &state); err != nil {
					return studioValidatedInput{}, err
				}
			}
			validatedPages = append(validatedPages, studioValidatedPage{
				ID:        page.ID,
				Title:     page.Title,
				PlainText: strings.TrimSpace(strings.Join(state.plain[pagePlainStart:], "\n")),
			})
		}
	} else {
		for _, raw := range content.Blocks {
			if err := state.validate(raw, 0); err != nil {
				return studioValidatedInput{}, err
			}
		}
	}
	if surface != "documents" && content.InfoCard != nil {
		return studioValidatedInput{}, fmt.Errorf("infoCard: unsupported for surface")
	}
	if surface == "documents" {
		if err := validateStudioInfoCard(content.InfoCard, &state); err != nil {
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
	brokenPageLinks := make([]string, 0, len(state.brokenPageLinks))
	for id := range state.brokenPageLinks {
		brokenPageLinks = append(brokenPageLinks, id)
	}
	sort.Strings(brokenPageLinks)
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
	if len(normalizedContent) > studioMaxRequest {
		return studioValidatedInput{}, fmt.Errorf("content: normalized content too large")
	}
	in.Content = normalizedContent
	return studioValidatedInput{
		Input: in, Content: content, Classification: classification,
		PlainText: plain, Pages: validatedPages,
		Links: links, BrokenPageLinks: brokenPageLinks,
		Assets: assets, Facets: facets,
	}, nil
}

func clampStudioFocus(value *int) *int {
	if value == nil {
		return nil
	}
	clamped := *value
	if clamped < 0 {
		clamped = 0
	}
	if clamped > 100 {
		clamped = 100
	}
	return &clamped
}

func validateStudioInfoCard(card *StudioInfoCard, state *studioBlockValidation) error {
	if card == nil {
		return nil
	}
	card.AssetID = strings.TrimSpace(card.AssetID)
	card.Caption = strings.TrimSpace(card.Caption)
	if card.AssetID != "" {
		if !studioIDRE.MatchString(card.AssetID) {
			return fmt.Errorf("infoCard.assetId: invalid")
		}
		state.assets[card.AssetID] = true
	}
	card.ImageRatio = strings.TrimSpace(card.ImageRatio)
	if card.ImageRatio != "" && !studioInfoRatios[card.ImageRatio] {
		return fmt.Errorf("infoCard.imageRatio: invalid")
	}
	// El punto focal es cosmético: se recorta al rango válido en vez de
	// rechazar el documento entero.
	card.ImageFocusX = clampStudioFocus(card.ImageFocusX)
	card.ImageFocusY = clampStudioFocus(card.ImageFocusY)
	if utf8.RuneCountInString(card.Caption) > 1000 {
		return fmt.Errorf("infoCard.caption: too long")
	}
	if card.Caption != "" {
		if err := state.addText(card.Caption, true, true); err != nil {
			return fmt.Errorf("infoCard.caption: %w", err)
		}
	}
	if len(card.Rows) > studioMaxInfoRows {
		return fmt.Errorf("infoCard.rows: too many")
	}
	for index := range card.Rows {
		row := &card.Rows[index]
		row.Label = strings.TrimSpace(row.Label)
		row.Value = strings.TrimSpace(row.Value)
		if utf8.RuneCountInString(row.Label) > 120 ||
			utf8.RuneCountInString(row.Value) > 4000 {
			return fmt.Errorf("infoCard.rows: value too long")
		}
		for _, value := range []string{row.Label, row.Value} {
			if value == "" {
				continue
			}
			if err := state.addText(value, true, true); err != nil {
				return fmt.Errorf("infoCard.rows: %w", err)
			}
		}
	}
	return nil
}

func normalizeStudioContentVersion(
	surface string,
	documentTitle string,
	content StudioContent,
) (StudioContent, error) {
	switch content.SchemaVersion {
	case studioLegacySchema:
		if surface != "documents" {
			if content.Blocks == nil {
				content.Blocks = []json.RawMessage{}
			}
			return content, nil
		}
		blocks := content.Blocks
		if blocks == nil {
			blocks = []json.RawMessage{}
		}
		content.SchemaVersion = studioSchemaVersion
		content.Blocks = nil
		content.Pages = []StudioPage{{
			ID:     "p1",
			Title:  strings.TrimSpace(documentTitle),
			Blocks: blocks,
		}}
		return content, nil
	case studioSchemaVersion:
		if surface != "documents" {
			return StudioContent{}, fmt.Errorf("schemaVersion: unsupported for surface")
		}
		if len(content.Blocks) != 0 {
			return StudioContent{}, fmt.Errorf("blocks: root blocks unsupported in schemaVersion 2")
		}
		for index := range content.Pages {
			if content.Pages[index].Blocks == nil {
				content.Pages[index].Blocks = []json.RawMessage{}
			}
		}
		return content, nil
	default:
		return StudioContent{}, fmt.Errorf("schemaVersion: unsupported")
	}
}

func normalizeStudioDocumentContent(document *StudioDocument) error {
	if document == nil || len(document.Content) == 0 {
		return nil
	}
	var content StudioContent
	if err := json.Unmarshal(document.Content, &content); err != nil {
		return fmt.Errorf("content: %w", err)
	}
	surface, ok := studioSurfaceForTemplate(document.TemplateKey)
	if !ok {
		return fmt.Errorf("templateKey: unsupported")
	}
	content, err := normalizeStudioContentVersion(surface, document.Title, content)
	if err != nil {
		return err
	}
	normalized, err := json.Marshal(content)
	if err != nil {
		return err
	}
	document.Content = normalized
	return nil
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
	ids             map[string]bool
	links           map[string]bool
	pageIDs         map[string]bool
	brokenPageLinks map[string]bool
	assets          map[string]bool
	count           int
	runes           int
	plain           []string
}

func (s *studioBlockValidation) inlinePlainText(value string) (string, error) {
	matches := studioInlineLinkRE.FindAllStringSubmatch(value, -1)
	for _, match := range matches {
		switch match[1] {
		case "page":
			if !s.pageIDs[match[2]] {
				s.brokenPageLinks[match[2]] = true
			}
		case "item":
			s.links[match[2]] = true
		}
	}
	remaining := studioInlineLinkRE.ReplaceAllString(value, "")
	if strings.Contains(remaining, "[[page:") || strings.Contains(remaining, "[[item:") {
		return "", fmt.Errorf("inline link: invalid syntax")
	}
	return studioInlineLinkRE.ReplaceAllString(value, "$3"), nil
}

func (s *studioBlockValidation) addText(value string, includePlain, parseLinks bool) error {
	s.runes += utf8.RuneCountInString(value)
	plain := value
	var err error
	if parseLinks {
		plain, err = s.inlinePlainText(value)
		if err != nil {
			return err
		}
	}
	if includePlain && strings.TrimSpace(plain) != "" {
		s.plain = append(s.plain, strings.TrimSpace(plain))
	}
	return nil
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
	if field, ok := obj["fontSize"]; ok {
		var size int
		if err := json.Unmarshal(field, &size); err != nil || size < 10 || size > 96 {
			return fmt.Errorf("block.fontSize: invalid")
		}
	}
	if field, ok := obj["textAlign"]; ok {
		var align string
		if err := json.Unmarshal(field, &align); err != nil ||
			(align != "left" && align != "center" && align != "right") {
			return fmt.Errorf("block.textAlign: invalid")
		}
	}
	for _, key := range []string{"text", "caption", "alt", "sideText", "title", "titleSnapshot"} {
		if field, ok := obj[key]; ok {
			var text string
			if err := json.Unmarshal(field, &text); err != nil {
				return fmt.Errorf("block.%s: string required", key)
			}
			n := utf8.RuneCountInString(text)
			if n > 100000 {
				return fmt.Errorf("block.%s: too long", key)
			}
			if err := s.addText(text, key != "alt", !(typ == "code" && key == "text")); err != nil {
				return fmt.Errorf("block.%s: %w", key, err)
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
			if err := s.addText(item, true, true); err != nil {
				return fmt.Errorf("block.items: %w", err)
			}
		}
	}
	if typ == "image" {
		var assetID string
		if err := json.Unmarshal(obj["assetId"], &assetID); err != nil || !studioIDRE.MatchString(assetID) {
			return fmt.Errorf("image.assetId: invalid")
		}
		if field, ok := obj["imageSize"]; ok {
			var size string
			if err := json.Unmarshal(field, &size); err != nil ||
				(size != "original" && size != "medium" && size != "small" && size != "poster") {
				return fmt.Errorf("image.imageSize: invalid")
			}
		}
		if field, ok := obj["imageAlign"]; ok {
			var align string
			if err := json.Unmarshal(field, &align); err != nil ||
				(align != "left" && align != "center" && align != "right") {
				return fmt.Errorf("image.imageAlign: invalid")
			}
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
			if err := s.addText(cell, true, true); err != nil {
				return fmt.Errorf("table.cell: %w", err)
			}
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
