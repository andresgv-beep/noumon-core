package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var errStudioMediaIncomplete = errors.New("studio media publication incomplete")

const (
	studioMaxMediaTracks    = 200
	studioMaxMediaSubtitles = 32
	studioMaxMediaChapters  = 500
	studioMaxDepotFiles     = 64
	studioMaxDepotShots     = 24
)

// StudioMediaMetadata is the neutral package contract shared by Cabinet and
// Moments. Every asset ID points to private Studio storage until publication.
type StudioMediaMetadata struct {
	Collection           string                `json:"collection,omitempty"`
	Date                 string                `json:"date,omitempty"`
	Contributor          string                `json:"contributor,omitempty"`
	License              string                `json:"license,omitempty"`
	PrimaryAssetID       string                `json:"primaryAssetId,omitempty"`
	PrimaryName          string                `json:"primaryName,omitempty"`
	CoverAssetID         string                `json:"coverAssetId,omitempty"`
	CoverName            string                `json:"coverName,omitempty"`
	ChannelAvatarAssetID string                `json:"channelAvatarAssetId,omitempty"`
	ChannelAvatarName    string                `json:"channelAvatarName,omitempty"`
	Duration             int                   `json:"duration,omitempty"`
	Tracks               []StudioMediaTrack    `json:"tracks,omitempty"`
	Subtitles            []StudioMediaSubtitle `json:"subtitles,omitempty"`
	Chapters             []StudioMediaChapter  `json:"chapters,omitempty"`

	// Depot (depot.program): la ficha de un programa. El icono es CoverAssetID.
	Version      string            `json:"version,omitempty"`
	Shelf        string            `json:"shelf,omitempty"` // estante: multimedia, oficina…
	Website      string            `json:"website,omitempty"`
	Requirements string            `json:"requirements,omitempty"`
	Languages    string            `json:"languages,omitempty"`
	Notes        string            `json:"notes,omitempty"` // novedades de esta versión
	Files        []StudioDepotFile `json:"files,omitempty"`
	Screenshots  []StudioDepotShot `json:"screenshots,omitempty"`
}

// StudioDepotFile: un archivo descargable del programa. Nombre, tamaño, tipo y
// SHA-256 salen del asset; aquí solo va lo que el autor decide.
type StudioDepotFile struct {
	AssetID string `json:"assetId"`
	OS      string `json:"os"`   // win | linux | mac
	Arch    string `json:"arch"` // x64 | arm64 | x86
	Label   string `json:"label,omitempty"`
	// Name: el nombre del fichero tal como se subió, solo para enseñarlo en el
	// editor (como primaryName). Al publicar manda el del asset.
	Name string `json:"name,omitempty"`
}

type StudioDepotShot struct {
	AssetID string `json:"assetId"`
	Caption string `json:"caption,omitempty"`
}

var (
	studioDepotOS   = map[string]bool{"win": true, "linux": true, "mac": true}
	studioDepotArch = map[string]bool{"x64": true, "arm64": true, "x86": true}
)

type StudioMediaTrack struct {
	Title           string `json:"title"`
	AssetID         string `json:"assetId"`
	WaveformAssetID string `json:"waveformAssetId,omitempty"`
}

type StudioMediaSubtitle struct {
	Lang    string `json:"lang"`
	AssetID string `json:"assetId"`
}

type StudioMediaChapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

func validateStudioMediaMetadata(template string, raw json.RawMessage) (StudioMediaMetadata, []string, error) {
	if !studioIsMediaTemplate(template) {
		return StudioMediaMetadata{}, nil, nil
	}
	var metadata StudioMediaMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return metadata, nil, fmt.Errorf("metadata: invalid media profile")
	}
	metadata.Collection = strings.TrimSpace(metadata.Collection)
	if metadata.Collection == "" {
		metadata.Collection = "General"
	}
	if utf8.RuneCountInString(metadata.Collection) > 120 || sanitizeSegment(metadata.Collection) == "" {
		return metadata, nil, fmt.Errorf("metadata.collection: invalid")
	}
	metadata.Collection = sanitizeSegment(metadata.Collection)
	for name, value := range map[string]string{
		"version": metadata.Version, "shelf": metadata.Shelf, "website": metadata.Website,
		"requirements": metadata.Requirements, "languages": metadata.Languages,
		"date": metadata.Date, "contributor": metadata.Contributor, "license": metadata.License,
		"primaryName": metadata.PrimaryName, "coverName": metadata.CoverName,
		"channelAvatarName": metadata.ChannelAvatarName,
	} {
		if utf8.RuneCountInString(strings.TrimSpace(value)) > 500 {
			return metadata, nil, fmt.Errorf("metadata.%s: too long", name)
		}
	}
	metadata.Version = strings.TrimSpace(metadata.Version)
	metadata.Shelf = strings.TrimSpace(metadata.Shelf)
	metadata.Website = strings.TrimSpace(metadata.Website)
	metadata.Requirements = strings.TrimSpace(metadata.Requirements)
	metadata.Languages = strings.TrimSpace(metadata.Languages)
	metadata.Notes = strings.TrimSpace(metadata.Notes)
	if utf8.RuneCountInString(metadata.Notes) > 4000 {
		return metadata, nil, fmt.Errorf("metadata.notes: too long")
	}
	metadata.Date = strings.TrimSpace(metadata.Date)
	metadata.Contributor = strings.TrimSpace(metadata.Contributor)
	metadata.License = strings.TrimSpace(metadata.License)
	metadata.PrimaryName = strings.TrimSpace(metadata.PrimaryName)
	metadata.CoverName = strings.TrimSpace(metadata.CoverName)
	metadata.ChannelAvatarName = strings.TrimSpace(metadata.ChannelAvatarName)
	// Cabinet Audio used to store one file separately as primaryAssetId while
	// the player only enumerated tracks. Normalize that legacy shape so the
	// former primary file becomes track 1 and every audio follows one ordering.
	if template == "cabinet.audio" && strings.TrimSpace(metadata.PrimaryAssetID) != "" {
		primaryID := strings.TrimSpace(metadata.PrimaryAssetID)
		index := -1
		for i := range metadata.Tracks {
			if strings.TrimSpace(metadata.Tracks[i].AssetID) == primaryID {
				index = i
				break
			}
		}
		var primaryTrack StudioMediaTrack
		if index >= 0 {
			primaryTrack = metadata.Tracks[index]
			metadata.Tracks = append(metadata.Tracks[:index], metadata.Tracks[index+1:]...)
		} else {
			title := strings.TrimSpace(strings.TrimSuffix(
				metadata.PrimaryName, filepath.Ext(metadata.PrimaryName)))
			if title == "" {
				title = "Audio"
			}
			primaryTrack = StudioMediaTrack{Title: title, AssetID: primaryID}
		}
		metadata.Tracks = append([]StudioMediaTrack{primaryTrack}, metadata.Tracks...)
		metadata.PrimaryAssetID = ""
		metadata.PrimaryName = ""
	}
	if metadata.Duration < 0 || metadata.Duration > 7*24*60*60 {
		return metadata, nil, fmt.Errorf("metadata.duration: invalid")
	}
	if len(metadata.Tracks) > studioMaxMediaTracks ||
		len(metadata.Subtitles) > studioMaxMediaSubtitles ||
		len(metadata.Chapters) > studioMaxMediaChapters ||
		len(metadata.Files) > studioMaxDepotFiles ||
		len(metadata.Screenshots) > studioMaxDepotShots {
		return metadata, nil, fmt.Errorf("metadata: too many entries")
	}

	assets := map[string]bool{}
	addAsset := func(field, id string, required bool) error {
		id = strings.TrimSpace(id)
		if id == "" && !required {
			return nil
		}
		if !studioIDRE.MatchString(id) {
			return fmt.Errorf("metadata.%s: invalid asset", field)
		}
		assets[id] = true
		return nil
	}
	if err := addAsset("primaryAssetId", metadata.PrimaryAssetID, false); err != nil {
		return metadata, nil, err
	}
	if err := addAsset("coverAssetId", metadata.CoverAssetID, false); err != nil {
		return metadata, nil, err
	}
	if err := addAsset("channelAvatarAssetId", metadata.ChannelAvatarAssetID, false); err != nil {
		return metadata, nil, err
	}
	for i := range metadata.Tracks {
		track := &metadata.Tracks[i]
		track.Title = strings.TrimSpace(track.Title)
		if track.Title == "" || utf8.RuneCountInString(track.Title) > 240 {
			return metadata, nil, fmt.Errorf("metadata.tracks[%d].title: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("tracks[%d].assetId", i), track.AssetID, true); err != nil {
			return metadata, nil, err
		}
		if err := addAsset(fmt.Sprintf("tracks[%d].waveformAssetId", i), track.WaveformAssetID, false); err != nil {
			return metadata, nil, err
		}
	}
	for i := range metadata.Subtitles {
		subtitle := &metadata.Subtitles[i]
		subtitle.Lang = strings.TrimSpace(subtitle.Lang)
		if subtitle.Lang == "" || utf8.RuneCountInString(subtitle.Lang) > 32 {
			return metadata, nil, fmt.Errorf("metadata.subtitles[%d].lang: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("subtitles[%d].assetId", i), subtitle.AssetID, true); err != nil {
			return metadata, nil, err
		}
	}
	previous := -1.0
	for i := range metadata.Chapters {
		chapter := &metadata.Chapters[i]
		chapter.Title = strings.TrimSpace(chapter.Title)
		if chapter.Start < 0 || chapter.Start < previous ||
			chapter.Title == "" || utf8.RuneCountInString(chapter.Title) > 240 {
			return metadata, nil, fmt.Errorf("metadata.chapters[%d]: invalid", i)
		}
		previous = chapter.Start
	}
	for i := range metadata.Files {
		file := &metadata.Files[i]
		file.OS = strings.TrimSpace(file.OS)
		file.Arch = strings.TrimSpace(file.Arch)
		file.Label = strings.TrimSpace(file.Label)
		file.Name = strings.TrimSpace(file.Name)
		if utf8.RuneCountInString(file.Name) > 255 {
			return metadata, nil, fmt.Errorf("metadata.files[%d].name: too long", i)
		}
		// El sistema puede quedar vacío en el borrador (un zip no dice para qué
		// es); publicar lo exige. Lo que no se admite es un valor inventado.
		if (file.OS != "" && !studioDepotOS[file.OS]) || !studioDepotArch[file.Arch] ||
			utf8.RuneCountInString(file.Label) > 120 {
			return metadata, nil, fmt.Errorf("metadata.files[%d]: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("files[%d].assetId", i), file.AssetID, true); err != nil {
			return metadata, nil, err
		}
	}
	for i := range metadata.Screenshots {
		shot := &metadata.Screenshots[i]
		shot.Caption = strings.TrimSpace(shot.Caption)
		if utf8.RuneCountInString(shot.Caption) > 240 {
			return metadata, nil, fmt.Errorf("metadata.screenshots[%d]: invalid", i)
		}
		if err := addAsset(fmt.Sprintf("screenshots[%d].assetId", i), shot.AssetID, true); err != nil {
			return metadata, nil, err
		}
	}
	depot := template == "depot.program"
	if !depot && (len(metadata.Files) != 0 || len(metadata.Screenshots) != 0) {
		return metadata, nil, fmt.Errorf("metadata.files: unsupported for template")
	}
	if depot && metadata.PrimaryAssetID != "" {
		return metadata, nil, fmt.Errorf("metadata.primaryAssetId: unsupported for template")
	}
	if template != "cabinet.audio" && len(metadata.Tracks) != 0 {
		return metadata, nil, fmt.Errorf("metadata.tracks: unsupported for template")
	}
	videoProfile := template == "moments.video" || template == "cabinet.video"
	if !videoProfile && len(metadata.Subtitles) != 0 {
		return metadata, nil, fmt.Errorf("metadata.subtitles: unsupported for template")
	}
	if !videoProfile && len(metadata.Chapters) != 0 {
		return metadata, nil, fmt.Errorf("metadata.chapters: unsupported for template")
	}
	out := make([]string, 0, len(assets))
	for id := range assets {
		out = append(out, id)
	}
	return metadata, out, nil
}

func studioMediaReadyForPublication(template string, metadata StudioMediaMetadata) error {
	if template == "depot.program" {
		if len(metadata.Files) == 0 {
			return fmt.Errorf("%w: at least one file required", errStudioMediaIncomplete)
		}
		for _, file := range metadata.Files {
			if file.OS == "" {
				return fmt.Errorf("%w: every file needs a system", errStudioMediaIncomplete)
			}
		}
		return nil
	}
	if metadata.PrimaryAssetID == "" && !(template == "cabinet.audio" && len(metadata.Tracks) > 0) {
		return fmt.Errorf("%w: primary file required", errStudioMediaIncomplete)
	}
	if template == "moments.video" && metadata.CoverAssetID == "" {
		return fmt.Errorf("%w: thumbnail required", errStudioMediaIncomplete)
	}
	return nil
}

// studioIsMediaTemplate: plantillas que publican fichero + ficha (Cabinet,
// Moments, Depot) en lugar de un documento de bloques.
func studioIsMediaTemplate(template string) bool {
	return strings.HasPrefix(template, "cabinet.") || template == "moments.video" ||
		template == "depot.program"
}
